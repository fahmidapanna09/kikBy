package stories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yourorg/chatplatform/internal/platform/ratelimit"
)

var (
	ErrInvalidStory   = errors.New("stories: invalid story")
	ErrInvalidMedia   = errors.New("stories: media is missing, not yours, not finished uploading, or the wrong kind")
	ErrInvalidMention = errors.New("stories: you can only mention people in your contacts")
	ErrForbidden      = errors.New("stories: only the author can do that")
	ErrRateLimited    = errors.New("stories: rate limited")
	ErrCannotMuteSelf = errors.New("stories: cannot mute yourself")
)

const (
	maxTextRunes    = 700
	maxCaptionRunes = 700
	maxReplyRunes   = 500
	maxAudience     = 500
	maxMentions     = 10
	defaultBgColor  = "#1F6F5C"
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// MediaVerifier is satisfied structurally by media.Service. It confirms the
// file belongs to userID and finished uploading, and reports what it is.
type MediaVerifier interface {
	VerifyOwnedUploaded(ctx context.Context, userID, mediaID string) (kind string, mimeType string, err error)
}

// EventPublisher is satisfied structurally by chat.Hub (live WebSocket push).
type EventPublisher interface {
	PublishEvent(ctx context.Context, userID, eventType string, payload any) error
}

// Notifier is satisfied structurally by notifications.Outbox (FCM push queue).
type Notifier interface {
	Enqueue(ctx context.Context, userID, kind string, payload json.RawMessage) error
}

type Service struct {
	repo     *Repository
	media    MediaVerifier
	events   EventPublisher
	notifier Notifier
	limiter  *ratelimit.Limiter
}

func NewService(repo *Repository, media MediaVerifier, events EventPublisher, notifier Notifier, limiter *ratelimit.Limiter) *Service {
	return &Service{repo: repo, media: media, events: events, notifier: notifier, limiter: limiter}
}

// normalizeCreate validates and cleans a create request in place. It is pure
// (no database or network) so the rules are unit-testable on their own.
func normalizeCreate(req *CreateStoryRequest, authorID string) error {
	bad := func(msg string) error { return fmt.Errorf("%w: %s", ErrInvalidStory, msg) }

	req.TextContent = strings.TrimSpace(req.TextContent)
	req.Caption = strings.TrimSpace(req.Caption)

	switch req.Type {
	case TypeText:
		if req.TextContent == "" {
			return bad("text stories need text")
		}
		if utf8.RuneCountInString(req.TextContent) > maxTextRunes {
			return bad("text is too long")
		}
		if req.MediaID != "" || req.Caption != "" {
			return bad("text stories can't have media or a caption")
		}
		if req.BackgroundColor == "" {
			req.BackgroundColor = defaultBgColor
		} else if !hexColor.MatchString(req.BackgroundColor) {
			return bad("background_color must look like #RRGGBB")
		}
	case TypeImage, TypeVideo:
		if req.MediaID == "" {
			return bad("media stories need a media_id")
		}
		if req.TextContent != "" || req.BackgroundColor != "" {
			return bad("media stories use a caption, not text_content or a background")
		}
		if utf8.RuneCountInString(req.Caption) > maxCaptionRunes {
			return bad("caption is too long")
		}
	default:
		return bad("type must be text, image, or video")
	}

	switch req.Privacy {
	case "":
		req.Privacy = PrivacyContacts
	case PrivacyContacts, PrivacyContactsExcept, PrivacySelected:
	default:
		return bad("privacy must be contacts, contacts_except, or selected")
	}

	req.AudienceUserIDs = uniqueWithout(req.AudienceUserIDs, authorID)
	if len(req.AudienceUserIDs) > maxAudience {
		return bad("audience list is too long")
	}
	switch req.Privacy {
	case PrivacyContacts:
		req.AudienceUserIDs = nil
	case PrivacySelected:
		if len(req.AudienceUserIDs) == 0 {
			return bad("pick at least one person to share with")
		}
	}

	req.MentionUserIDs = uniqueWithout(req.MentionUserIDs, authorID)
	if len(req.MentionUserIDs) > maxMentions {
		return bad("you can mention at most 10 people")
	}
	return nil
}

func uniqueWithout(ids []string, exclude string) []string {
	seen := map[string]bool{exclude: true}
	var out []string
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (s *Service) CreateStory(ctx context.Context, authorID string, req CreateStoryRequest) (Story, error) {
	if err := normalizeCreate(&req, authorID); err != nil {
		return Story{}, err
	}
	if ok, err := s.limiter.Allow(ctx, "story:post:"+authorID, 30, 24*time.Hour); err != nil {
		return Story{}, err
	} else if !ok {
		return Story{}, ErrRateLimited
	}

	if req.Type != TypeText {
		kind, _, err := s.media.VerifyOwnedUploaded(ctx, authorID, req.MediaID)
		if err != nil || kind != req.Type {
			return Story{}, ErrInvalidMedia
		}
	}

	if len(req.MentionUserIDs) > 0 {
		own, err := s.repo.FilterOwnContacts(ctx, authorID, req.MentionUserIDs)
		if err != nil {
			return Story{}, err
		}
		if len(own) != len(req.MentionUserIDs) {
			return Story{}, ErrInvalidMention
		}
	}

	story, err := s.repo.CreateStory(ctx, authorID, req, time.Now().Add(StoryLifetime))
	if err != nil {
		return Story{}, err
	}

	// Announce in the background: the audience can be large, and the author
	// shouldn't wait on it. Failures only mean someone sees the story on
	// their next feed refresh instead of instantly.
	go func(story Story, mentions []string) {
		bg, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s.announceCreated(bg, story, mentions)
	}(story, req.MentionUserIDs)

	return story, nil
}

func (s *Service) announceCreated(ctx context.Context, story Story, mentions []string) {
	if s.events != nil {
		if audience, err := s.repo.AudienceFor(ctx, story.ID, story.AuthorID); err == nil {
			payload := map[string]string{"story_id": story.ID, "author_id": story.AuthorID}
			for _, uid := range audience {
				_ = s.events.PublishEvent(ctx, uid, "story.created", payload)
			}
		}
	}
	if s.notifier != nil && len(mentions) > 0 {
		payload, _ := json.Marshal(map[string]string{"story_id": story.ID, "author_id": story.AuthorID})
		for _, uid := range mentions {
			_ = s.notifier.Enqueue(ctx, uid, "story.mention", payload)
		}
	}
}

func (s *Service) Feed(ctx context.Context, viewerID string) (FeedResponse, error) {
	rows, err := s.repo.Feed(ctx, viewerID)
	if err != nil {
		return FeedResponse{}, err
	}
	return FeedResponse{Entries: groupFeed(rows)}, nil
}

func (s *Service) Mine(ctx context.Context, authorID string) (MyStoriesResponse, error) {
	list, err := s.repo.MyActive(ctx, authorID)
	return MyStoriesResponse{Stories: list}, err
}

func (s *Service) Archive(ctx context.Context, authorID string, before *time.Time) (MyStoriesResponse, error) {
	list, err := s.repo.Archive(ctx, authorID, before, 30)
	return MyStoriesResponse{Stories: list}, err
}

func (s *Service) Delete(ctx context.Context, authorID, storyID string) error {
	return s.repo.SoftDelete(ctx, authorID, storyID)
}

// RecordView marks the story seen by viewerID and, the first time only, lets
// the author know. A story the viewer can't see is reported as not found —
// not "forbidden" — so its existence isn't revealed.
func (s *Service) RecordView(ctx context.Context, viewerID, storyID string) error {
	if err := s.requireVisible(ctx, viewerID, storyID); err != nil {
		return err
	}
	isNew, count, err := s.repo.RecordView(ctx, viewerID, storyID)
	if err != nil {
		return err
	}
	if isNew {
		s.notifyAuthorEvent(ctx, storyID, "story.viewed", map[string]any{"story_id": storyID, "viewer_id": viewerID, "view_count": count})
	}
	return nil
}

func (s *Service) Viewers(ctx context.Context, authorID, storyID string) (ViewersResponse, error) {
	if err := s.requireAuthor(ctx, authorID, storyID); err != nil {
		return ViewersResponse{}, err
	}
	v, err := s.repo.ListViewers(ctx, storyID)
	return ViewersResponse{Viewers: v}, err
}

func (s *Service) React(ctx context.Context, userID, storyID, emoji string) error {
	emoji = strings.TrimSpace(emoji)
	if emoji == "" || len(emoji) > 32 || !utf8.ValidString(emoji) {
		return fmt.Errorf("%w: emoji must be a short, valid string", ErrInvalidStory)
	}
	if err := s.requireVisible(ctx, userID, storyID); err != nil {
		return err
	}
	// Reacting counts as having viewed it, so the author's viewer list never
	// shows a reaction from someone it doesn't list.
	if _, _, err := s.repo.RecordView(ctx, userID, storyID); err != nil {
		return err
	}
	if err := s.repo.SetReaction(ctx, storyID, userID, emoji); err != nil {
		return err
	}
	s.notifyAuthorEvent(ctx, storyID, "story.reacted", map[string]any{"story_id": storyID, "user_id": userID, "emoji": emoji})
	return nil
}

func (s *Service) Unreact(ctx context.Context, userID, storyID string) error {
	if err := s.requireVisible(ctx, userID, storyID); err != nil {
		return err
	}
	return s.repo.RemoveReaction(ctx, storyID, userID)
}

func (s *Service) Reply(ctx context.Context, userID, storyID, body string) (Reply, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > maxReplyRunes {
		return Reply{}, fmt.Errorf("%w: a reply must be 1-%d characters", ErrInvalidStory, maxReplyRunes)
	}
	if ok, err := s.limiter.Allow(ctx, "story:reply:"+userID, 30, time.Hour); err != nil {
		return Reply{}, err
	} else if !ok {
		return Reply{}, ErrRateLimited
	}
	if err := s.requireVisible(ctx, userID, storyID); err != nil {
		return Reply{}, err
	}
	rep, err := s.repo.AddReply(ctx, storyID, userID, body)
	if err != nil {
		return Reply{}, err
	}
	s.notifyAuthorEvent(ctx, storyID, "story.replied", map[string]any{"story_id": storyID, "reply_id": rep.ID, "replier_id": userID})
	if s.notifier != nil {
		if author, err := s.repo.AuthorOf(ctx, storyID); err == nil {
			payload, _ := json.Marshal(map[string]string{"story_id": storyID, "reply_id": rep.ID})
			_ = s.notifier.Enqueue(ctx, author, "story.reply", payload)
		}
	}
	return rep, nil
}

func (s *Service) Replies(ctx context.Context, authorID, storyID string) (RepliesResponse, error) {
	if err := s.requireAuthor(ctx, authorID, storyID); err != nil {
		return RepliesResponse{}, err
	}
	list, err := s.repo.ListReplies(ctx, storyID)
	return RepliesResponse{Replies: list}, err
}

func (s *Service) Mute(ctx context.Context, muterID, mutedID string) error {
	if muterID == mutedID {
		return ErrCannotMuteSelf
	}
	return s.repo.Mute(ctx, muterID, mutedID)
}

func (s *Service) Unmute(ctx context.Context, muterID, mutedID string) error {
	return s.repo.Unmute(ctx, muterID, mutedID)
}

func (s *Service) requireVisible(ctx context.Context, viewerID, storyID string) error {
	ok, err := s.repo.CanView(ctx, viewerID, storyID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (s *Service) requireAuthor(ctx context.Context, userID, storyID string) error {
	author, err := s.repo.AuthorOf(ctx, storyID)
	if err != nil {
		return err
	}
	if author != userID {
		return ErrForbidden
	}
	return nil
}

func (s *Service) notifyAuthorEvent(ctx context.Context, storyID, eventType string, payload map[string]any) {
	if s.events == nil {
		return
	}
	if author, err := s.repo.AuthorOf(ctx, storyID); err == nil {
		_ = s.events.PublishEvent(ctx, author, eventType, payload)
	}
}
