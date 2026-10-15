package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yourorg/chatplatform/internal/platform/ratelimit"
)

var (
	ErrPermissionDenied  = errors.New("channels: only an admin of this channel can do that")
	ErrNotAdmin          = ErrPermissionDenied
	ErrNotFollowing      = errors.New("channels: you must follow this channel to do that")
	ErrInvalidPost       = errors.New("channels: invalid post")
	ErrInvalidMedia      = errors.New("channels: media is missing, not yours, not finished uploading, or the wrong kind for this post type")
	ErrInviteNotFound    = errors.New("channels: invite code not found or revoked")
	ErrRateLimited       = errors.New("channels: rate limited")
	ErrEmptyName         = errors.New("channels: name is required")
	ErrCannotDemoteOwner = errors.New("channels: the owner can't be removed as admin")
)

// MediaVerifier is satisfied structurally by media.Service — same contract
// stories.Service uses, so both modules share one implementation.
type MediaVerifier interface {
	VerifyOwnedUploaded(ctx context.Context, userID, mediaID string) (kind string, mimeType string, err error)
}

// EventPublisher/Notifier: satisfied structurally by chat.Hub and
// notifications.Outbox respectively — same contracts stories.Service uses.
type EventPublisher interface {
	PublishEvent(ctx context.Context, userID, eventType string, payload any) error
}
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

func (s *Service) CreateChannel(ctx context.Context, creatorID string, req CreateChannelRequest) (ChannelSummary, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return ChannelSummary{}, ErrEmptyName
	}
	req.Name = name
	c, err := s.repo.Create(ctx, creatorID, req)
	if err != nil {
		return ChannelSummary{}, err
	}
	// The creator follows their own channel by default, matching how a
	// group's creator is automatically a member — otherwise they wouldn't
	// see their own posts in a followed-channels list.
	_ = s.repo.Follow(ctx, c.ID, creatorID)
	return s.summary(ctx, c, creatorID)
}

func (s *Service) summary(ctx context.Context, c Channel, viewerID string) (ChannelSummary, error) {
	followerCount, err := s.repo.FollowerCount(ctx, c.ID)
	if err != nil {
		return ChannelSummary{}, err
	}
	isFollowing, err := s.repo.IsFollowing(ctx, c.ID, viewerID)
	if err != nil {
		return ChannelSummary{}, err
	}
	_, isAdmin, err := s.repo.AdminRole(ctx, c.ID, viewerID)
	if err != nil {
		return ChannelSummary{}, err
	}
	sum := ChannelSummary{
		ID: c.ID, Name: c.Name, Description: c.Description, IconMediaID: c.IconMediaID, Category: c.Category,
		IsPublic: c.IsPublic, FollowerCount: followerCount, IsFollowing: isFollowing, IsAdmin: isAdmin,
	}
	if isAdmin {
		sum.InviteCode = c.InviteCode
	}
	return sum, nil
}

func (s *Service) Get(ctx context.Context, viewerID, channelID string) (ChannelSummary, error) {
	c, err := s.repo.Get(ctx, channelID)
	if err != nil {
		return ChannelSummary{}, err
	}
	if !c.IsPublic {
		_, isAdmin, err := s.repo.AdminRole(ctx, channelID, viewerID)
		if err != nil {
			return ChannelSummary{}, err
		}
		following, err := s.repo.IsFollowing(ctx, channelID, viewerID)
		if err != nil {
			return ChannelSummary{}, err
		}
		if !isAdmin && !following {
			return ChannelSummary{}, ErrNotFound // don't reveal a private channel's existence
		}
	}
	return s.summary(ctx, c, viewerID)
}

func (s *Service) Update(ctx context.Context, userID, channelID string, req UpdateChannelRequest) (ChannelSummary, error) {
	if err := s.requireAdmin(ctx, userID, channelID); err != nil {
		return ChannelSummary{}, err
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			return ChannelSummary{}, ErrEmptyName
		}
		req.Name = &trimmed
	}
	if err := s.repo.Update(ctx, channelID, req); err != nil {
		return ChannelSummary{}, err
	}
	c, err := s.repo.Get(ctx, channelID)
	if err != nil {
		return ChannelSummary{}, err
	}
	return s.summary(ctx, c, userID)
}

func (s *Service) Delete(ctx context.Context, userID, channelID string) error {
	role, isAdmin, err := s.repo.AdminRole(ctx, channelID, userID)
	if err != nil {
		return err
	}
	if !isAdmin || role != RoleOwner {
		return ErrPermissionDenied
	}
	return s.repo.SoftDelete(ctx, channelID)
}

func (s *Service) requireAdmin(ctx context.Context, userID, channelID string) error {
	_, isAdmin, err := s.repo.AdminRole(ctx, channelID, userID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrPermissionDenied
	}
	return nil
}

// --- Admins ---

func (s *Service) AddAdmin(ctx context.Context, actingUserID, channelID, targetUserID string) error {
	if err := s.requireAdmin(ctx, actingUserID, channelID); err != nil {
		return err
	}
	return s.repo.AddAdmin(ctx, channelID, targetUserID)
}

func (s *Service) RemoveAdmin(ctx context.Context, actingUserID, channelID, targetUserID string) error {
	actingRole, isAdmin, err := s.repo.AdminRole(ctx, channelID, actingUserID)
	if err != nil {
		return err
	}
	if !isAdmin || actingRole != RoleOwner {
		return ErrPermissionDenied // only the owner manages the admin list
	}
	targetRole, targetIsAdmin, err := s.repo.AdminRole(ctx, channelID, targetUserID)
	if err != nil {
		return err
	}
	if targetIsAdmin && targetRole == RoleOwner {
		return ErrCannotDemoteOwner
	}
	return s.repo.RemoveAdmin(ctx, channelID, targetUserID)
}

func (s *Service) ListAdmins(ctx context.Context, userID, channelID string) ([]Admin, error) {
	if err := s.requireAdmin(ctx, userID, channelID); err != nil {
		return nil, err
	}
	return s.repo.ListAdmins(ctx, channelID)
}

// --- Follow ---

func (s *Service) Follow(ctx context.Context, userID, channelID string) (FollowResponse, error) {
	c, err := s.repo.Get(ctx, channelID)
	if err != nil {
		return FollowResponse{}, err
	}
	if !c.IsPublic {
		return FollowResponse{}, ErrNotFound // private channels are joined via invite link, not direct follow
	}
	if err := s.repo.Follow(ctx, channelID, userID); err != nil {
		return FollowResponse{}, err
	}
	return FollowResponse{ChannelID: channelID}, nil
}

func (s *Service) FollowByInvite(ctx context.Context, userID, inviteCode string) (FollowResponse, error) {
	c, err := s.repo.GetByInviteCode(ctx, inviteCode)
	if err != nil {
		return FollowResponse{}, err
	}
	if err := s.repo.Follow(ctx, c.ID, userID); err != nil {
		return FollowResponse{}, err
	}
	return FollowResponse{ChannelID: c.ID}, nil
}

func (s *Service) Unfollow(ctx context.Context, userID, channelID string) error {
	return s.repo.Unfollow(ctx, channelID, userID)
}

func (s *Service) SetMuted(ctx context.Context, userID, channelID string, muted bool) error {
	return s.repo.SetFollowMuted(ctx, channelID, userID, muted)
}

func (s *Service) Mine(ctx context.Context, userID string) ([]ChannelSummary, error) {
	list, err := s.repo.ListFollowed(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelSummary, 0, len(list))
	for _, c := range list {
		sum, err := s.summary(ctx, c, userID)
		if err != nil {
			return nil, err
		}
		out = append(out, sum)
	}
	return out, nil
}

// --- Posts ---

const (
	maxPostBodyRunes = 2000
	minPollOptions   = 2
	maxPollOptions   = 10
)

func (s *Service) CreatePost(ctx context.Context, authorID, channelID string, req CreatePostRequest) (Post, error) {
	if err := s.requireAdmin(ctx, authorID, channelID); err != nil {
		return Post{}, err
	}
	if ok, err := s.limiter.Allow(ctx, "channel:post:"+authorID, 50, 24*time.Hour); err != nil { // 50 posts/day
		return Post{}, err
	} else if !ok {
		return Post{}, ErrRateLimited
	}

	req.Body = strings.TrimSpace(req.Body)
	if utf8.RuneCountInString(req.Body) > maxPostBodyRunes {
		return Post{}, fmt.Errorf("%w: body is too long", ErrInvalidPost)
	}

	switch req.Type {
	case PostText:
		if req.Body == "" {
			return Post{}, fmt.Errorf("%w: text posts need a body", ErrInvalidPost)
		}
	case PostImage, PostVideo, PostAudio, PostSticker:
		if req.MediaID == "" {
			return Post{}, fmt.Errorf("%w: this post type needs media_id", ErrInvalidPost)
		}
		kind, _, err := s.media.VerifyOwnedUploaded(ctx, authorID, req.MediaID)
		if err != nil || kind != req.Type {
			// A sticker is stored as an image upload (see media allowlist) —
			// accept kind "image" for post type "sticker" specifically.
			if !(req.Type == PostSticker && kind == PostImage && err == nil) {
				return Post{}, ErrInvalidMedia
			}
		}
	case PostLink:
		if req.LinkURL == "" {
			return Post{}, fmt.Errorf("%w: link posts need link_url", ErrInvalidPost)
		}
	case PostPoll:
		if len(req.PollOptions) < minPollOptions || len(req.PollOptions) > maxPollOptions {
			return Post{}, fmt.Errorf("%w: a poll needs %d-%d options", ErrInvalidPost, minPollOptions, maxPollOptions)
		}
		if req.Body == "" {
			return Post{}, fmt.Errorf("%w: a poll needs a question in body", ErrInvalidPost)
		}
		for i, opt := range req.PollOptions {
			trimmed := strings.TrimSpace(opt)
			if trimmed == "" {
				return Post{}, fmt.Errorf("%w: poll options can't be empty", ErrInvalidPost)
			}
			req.PollOptions[i] = trimmed
		}
	default:
		return Post{}, fmt.Errorf("%w: unknown post type %q", ErrInvalidPost, req.Type)
	}

	post, err := s.repo.CreatePost(ctx, channelID, authorID, req)
	if err != nil {
		return Post{}, err
	}
	post = s.enrich(ctx, post, authorID)

	go func(p Post) {
		ctx2 := context.Background()
		s.announcePost(ctx2, p)
	}(post)

	return post, nil
}

func (s *Service) announcePost(ctx context.Context, post Post) {
	followerIDs, err := s.repo.ListFollowerIDs(ctx, post.ChannelID, false, true)
	if err != nil {
		return
	}
	payload := map[string]string{"channel_id": post.ChannelID, "post_id": post.ID}
	for _, uid := range followerIDs {
		if uid == post.AuthorID {
			continue
		}
		if s.events != nil {
			_ = s.events.PublishEvent(ctx, uid, "channel.post", payload)
		}
		if s.notifier != nil {
			data, _ := json.Marshal(payload)
			_ = s.notifier.Enqueue(ctx, uid, "channel.update", data)
		}
	}
}

// enrich fills in view count, reactions, and (for polls) option tallies —
// split out from CreatePost/ListPosts's query so both share one definition
// of "what a post looks like once returned to a client".
func (s *Service) enrich(ctx context.Context, p Post, viewerID string) Post {
	if n, err := s.repo.ViewCount(ctx, p.ID); err == nil {
		p.ViewCount = n
	}
	if rs, err := s.repo.ReactionSummaries(ctx, p.ID); err == nil {
		p.Reactions = rs
	}
	if p.Type == PostPoll {
		if opts, err := s.repo.PollOptions(ctx, p.ID, viewerID); err == nil {
			p.PollOptions = opts
		}
	}
	return p
}

func (s *Service) EditPost(ctx context.Context, userID, postID, body string) (Post, error) {
	post, err := s.requireAuthorOrOwner(ctx, userID, postID)
	if err != nil {
		return Post{}, err
	}
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > maxPostBodyRunes {
		return Post{}, fmt.Errorf("%w: body must be 1-%d characters", ErrInvalidPost, maxPostBodyRunes)
	}
	if err := s.repo.EditPost(ctx, postID, body); err != nil {
		return Post{}, err
	}
	updated, err := s.repo.GetPost(ctx, postID)
	if err != nil {
		return Post{}, err
	}
	return s.enrich(ctx, updated, post.AuthorID), nil
}

func (s *Service) DeletePost(ctx context.Context, userID, postID string) error {
	if _, err := s.requireAuthorOrOwner(ctx, userID, postID); err != nil {
		return err
	}
	return s.repo.DeletePost(ctx, postID)
}

// requireAuthorOrOwner: the post's own author, or the channel owner, may
// edit/delete it — an ordinary admin can't edit someone else's post.
func (s *Service) requireAuthorOrOwner(ctx context.Context, userID, postID string) (Post, error) {
	post, err := s.repo.GetPost(ctx, postID)
	if err != nil {
		return Post{}, err
	}
	if post.AuthorID == userID {
		return post, nil
	}
	role, isAdmin, err := s.repo.AdminRole(ctx, post.ChannelID, userID)
	if err != nil {
		return Post{}, err
	}
	if isAdmin && role == RoleOwner {
		return post, nil
	}
	return Post{}, ErrPermissionDenied
}

func (s *Service) ListPosts(ctx context.Context, viewerID, channelID string, beforeSeq int64) (PostsResponse, error) {
	if err := s.requireCanRead(ctx, viewerID, channelID); err != nil {
		return PostsResponse{}, err
	}
	posts, err := s.repo.ListPosts(ctx, channelID, beforeSeq, 30)
	if err != nil {
		return PostsResponse{}, err
	}
	for i := range posts {
		posts[i] = s.enrich(ctx, posts[i], viewerID)
	}
	resp := PostsResponse{Posts: posts}
	if len(posts) == 30 {
		resp.NextCursor = fmt.Sprintf("%d", posts[len(posts)-1].Seq)
	}
	return resp, nil
}

func (s *Service) requireCanRead(ctx context.Context, viewerID, channelID string) error {
	c, err := s.repo.Get(ctx, channelID)
	if err != nil {
		return err
	}
	if c.IsPublic {
		return nil
	}
	_, isAdmin, err := s.repo.AdminRole(ctx, channelID, viewerID)
	if err != nil {
		return err
	}
	if isAdmin {
		return nil
	}
	following, err := s.repo.IsFollowing(ctx, channelID, viewerID)
	if err != nil {
		return err
	}
	if !following {
		return ErrNotFound
	}
	return nil
}

func (s *Service) RecordView(ctx context.Context, userID, postID string) error {
	channelID, err := s.repo.ChannelOfPost(ctx, postID)
	if err != nil {
		return err
	}
	if err := s.requireCanRead(ctx, userID, channelID); err != nil {
		return err
	}
	return s.repo.RecordView(ctx, postID, userID)
}

func (s *Service) React(ctx context.Context, userID, postID, emoji string) error {
	channelID, err := s.repo.ChannelOfPost(ctx, postID)
	if err != nil {
		return err
	}
	if err := s.requireCanRead(ctx, userID, channelID); err != nil {
		return err
	}
	if emoji == "" || len(emoji) > 32 {
		return fmt.Errorf("%w: emoji must be a short, non-empty string", ErrInvalidPost)
	}
	return s.repo.SetReaction(ctx, postID, userID, emoji)
}

func (s *Service) Unreact(ctx context.Context, userID, postID string) error {
	return s.repo.RemoveReaction(ctx, postID, userID)
}

func (s *Service) Vote(ctx context.Context, userID, postID, optionID string) error {
	channelID, err := s.repo.ChannelOfPost(ctx, postID)
	if err != nil {
		return err
	}
	if err := s.requireCanRead(ctx, userID, channelID); err != nil {
		return err
	}
	ok, err := s.repo.OptionBelongsToPost(ctx, postID, optionID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: that option doesn't belong to this poll", ErrInvalidPost)
	}
	return s.repo.Vote(ctx, postID, optionID, userID)
}

// --- Invite links ---

func (s *Service) GenerateInviteLink(ctx context.Context, userID, channelID string) (string, error) {
	if err := s.requireAdmin(ctx, userID, channelID); err != nil {
		return "", err
	}
	return s.repo.SetNewInviteCode(ctx, channelID)
}

func (s *Service) RevokeInviteLink(ctx context.Context, userID, channelID string) error {
	if err := s.requireAdmin(ctx, userID, channelID); err != nil {
		return err
	}
	return s.repo.RevokeInviteCode(ctx, channelID)
}

// --- Search & reports ---

func (s *Service) Search(ctx context.Context, query, category string) (SearchResponse, error) {
	list, err := s.repo.SearchPublic(ctx, strings.TrimSpace(query), strings.TrimSpace(category), 20)
	if err != nil {
		return SearchResponse{}, err
	}
	out := make([]ChannelSummary, 0, len(list))
	for _, c := range list {
		// Search is anonymous-ish from the summary's point of view: no
		// specific viewer, so is_following/is_admin are always false and
		// the invite code is never included here.
		out = append(out, ChannelSummary{
			ID: c.ID, Name: c.Name, Description: c.Description, IconMediaID: c.IconMediaID,
			Category: c.Category, IsPublic: c.IsPublic,
		})
	}
	// Follower counts still matter for search result ranking/display.
	for i := range out {
		if n, err := s.repo.FollowerCount(ctx, out[i].ID); err == nil {
			out[i].FollowerCount = n
		}
	}
	return SearchResponse{Channels: out}, nil
}

func (s *Service) Report(ctx context.Context, reporterID, channelID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("%w: reason is required", ErrInvalidPost)
	}
	return s.repo.Report(ctx, reporterID, channelID, reason)
}

// --- Phase 11 admin surface. No authorization check here, by design —
// the caller (internal/admin) has already verified the acting user holds
// an admin role before reaching this method, same as chat.Service's
// ForceAddMember/ForceRemoveMember trust communities.Service's prior
// membership-role check.

func (s *Service) ListReportsAdmin(ctx context.Context, onlyUnresolved bool, limit, offset int) ([]map[string]any, int, error) {
	rows, total, err := s.repo.ListReports(ctx, onlyUnresolved, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]map[string]any, len(rows))
	for i, rr := range rows {
		out[i] = map[string]any{
			"id": rr.ID, "reporter_id": rr.ReporterID, "channel_id": rr.ChannelID,
			"channel_name": rr.ChannelName, "reason": rr.Reason, "created_at": rr.CreatedAt,
			"resolved_at": rr.ResolvedAt, "resolved_by": rr.ResolvedBy, "resolution_note": rr.ResolutionNote,
		}
	}
	return out, total, nil
}

func (s *Service) ResolveReportAdmin(ctx context.Context, reportID, resolvedByUserID, note string) error {
	return s.repo.ResolveReport(ctx, reportID, resolvedByUserID, note)
}
