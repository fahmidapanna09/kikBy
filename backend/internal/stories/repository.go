package stories

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("stories: not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// visibleTemplate is the ONE definition of "may this viewer see story s
// right now". Feed, single-story checks, story-media downloads and the
// fan-out audience all build their query from it, so they can't drift apart.
// {viewer} is substituted with either a bind parameter ("$1") or a column
// (e.g. "cand.id"); the story must be aliased as `s`.
//
// Rules: not deleted, not expired, not your own, no block in either
// direction, and then at least one of — you're mentioned; privacy=contacts
// and the author has you as a contact; privacy=contacts_except, the author
// has you as a contact and didn't exclude you; privacy=selected and you're on
// the include list.
const visibleTemplate = `
	s.deleted_at IS NULL AND s.expires_at > now()
	AND s.author_id <> {viewer}
	AND NOT EXISTS (
		SELECT 1 FROM blocked_users b
		WHERE (b.blocker_id = {viewer} AND b.blocked_id = s.author_id)
		   OR (b.blocker_id = s.author_id AND b.blocked_id = {viewer})
	)
	AND (
		EXISTS (SELECT 1 FROM story_mentions m WHERE m.story_id = s.id AND m.user_id = {viewer})
		OR (s.privacy = 'contacts'
			AND EXISTS (SELECT 1 FROM contacts c WHERE c.owner_id = s.author_id AND c.contact_user_id = {viewer}))
		OR (s.privacy = 'contacts_except'
			AND EXISTS (SELECT 1 FROM contacts c WHERE c.owner_id = s.author_id AND c.contact_user_id = {viewer})
			AND NOT EXISTS (SELECT 1 FROM story_audience a WHERE a.story_id = s.id AND a.user_id = {viewer} AND a.kind = 'exclude'))
		OR (s.privacy = 'selected'
			AND EXISTS (SELECT 1 FROM story_audience a WHERE a.story_id = s.id AND a.user_id = {viewer} AND a.kind = 'include'))
	)`

func visibleSQL(viewerExpr string) string {
	return strings.ReplaceAll(visibleTemplate, "{viewer}", viewerExpr)
}

const storyCols = `s.id, s.author_id, s.story_type, s.text_content, s.background_color, s.caption, s.media_id, s.created_at, s.expires_at`

// CreateStory inserts the story plus its audience and mention rows in one
// transaction — a story is never half-created with the wrong audience.
func (r *Repository) CreateStory(ctx context.Context, authorID string, req CreateStoryRequest, expiresAt time.Time) (Story, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Story{}, err
	}
	defer tx.Rollback(ctx)

	nullable := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}

	story := Story{AuthorID: authorID, Type: req.Type, Privacy: req.Privacy, ExpiresAt: expiresAt}
	story.TextContent = nullable(req.TextContent)
	story.BackgroundColor = nullable(req.BackgroundColor)
	story.Caption = nullable(req.Caption)
	story.MediaID = nullable(req.MediaID)

	err = tx.QueryRow(ctx, `
		INSERT INTO stories (author_id, story_type, text_content, background_color, caption, media_id, privacy, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`, authorID, req.Type, story.TextContent, story.BackgroundColor, story.Caption, story.MediaID, req.Privacy, expiresAt,
	).Scan(&story.ID, &story.CreatedAt)
	if err != nil {
		return Story{}, err
	}

	kind := ""
	switch req.Privacy {
	case PrivacyContactsExcept:
		kind = "exclude"
	case PrivacySelected:
		kind = "include"
	}
	if kind != "" {
		for _, uid := range req.AudienceUserIDs {
			if _, err := tx.Exec(ctx,
				`INSERT INTO story_audience (story_id, user_id, kind) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
				story.ID, uid, kind,
			); err != nil {
				return Story{}, err
			}
		}
	}
	for _, uid := range req.MentionUserIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO story_mentions (story_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			story.ID, uid,
		); err != nil {
			return Story{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Story{}, err
	}
	return story, nil
}

// FilterOwnContacts returns the subset of ids that are registered contacts of
// ownerID — used to make sure you can only @mention people in your contacts.
func (r *Repository) FilterOwnContacts(ctx context.Context, ownerID string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT contact_user_id FROM contacts WHERE owner_id = $1 AND contact_user_id = ANY($2)`, ownerID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repository) Feed(ctx context.Context, viewerID string) ([]feedRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+storyCols+`,
		       EXISTS (SELECT 1 FROM story_views v WHERE v.story_id = s.id AND v.viewer_id = $1) AS seen,
		       EXISTS (SELECT 1 FROM story_mutes mu WHERE mu.muter_id = $1 AND mu.muted_id = s.author_id) AS muted,
		       u.username, p.display_name
		FROM stories s
		JOIN users u ON u.id = s.author_id
		JOIN user_profiles p ON p.user_id = s.author_id
		WHERE `+visibleSQL("$1")+`
		ORDER BY s.author_id, s.created_at ASC
		LIMIT 500
	`, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []feedRow
	for rows.Next() {
		var fr feedRow
		st := &fr.Story
		if err := rows.Scan(&st.ID, &st.AuthorID, &st.Type, &st.TextContent, &st.BackgroundColor, &st.Caption, &st.MediaID,
			&st.CreatedAt, &st.ExpiresAt, &st.Seen, &fr.Muted, &fr.Username, &fr.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, fr)
	}
	return out, rows.Err()
}

func (r *Repository) ownStories(ctx context.Context, where string, args ...any) ([]Story, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+storyCols+`, s.privacy,
		       (SELECT COUNT(*) FROM story_views v WHERE v.story_id = s.id),
		       (SELECT COUNT(*) FROM story_reactions rc WHERE rc.story_id = s.id),
		       (SELECT COUNT(*) FROM story_replies rp WHERE rp.story_id = s.id)
		FROM stories s
		WHERE s.deleted_at IS NULL AND `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Story
	for rows.Next() {
		var st Story
		if err := rows.Scan(&st.ID, &st.AuthorID, &st.Type, &st.TextContent, &st.BackgroundColor, &st.Caption, &st.MediaID,
			&st.CreatedAt, &st.ExpiresAt, &st.Privacy, &st.ViewCount, &st.ReactionCount, &st.ReplyCount); err != nil {
			return nil, err
		}
		st.Seen = true
		out = append(out, st)
	}
	return out, rows.Err()
}

func (r *Repository) MyActive(ctx context.Context, authorID string) ([]Story, error) {
	return r.ownStories(ctx, `s.author_id = $1 AND s.expires_at > now() ORDER BY s.created_at ASC`, authorID)
}

// Archive returns the author's expired (not deleted) stories, newest first.
// before is an exclusive created_at cursor for paging; nil starts from the newest.
func (r *Repository) Archive(ctx context.Context, authorID string, before *time.Time, limit int) ([]Story, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	return r.ownStories(ctx,
		`s.author_id = $1 AND s.expires_at <= now() AND ($2::timestamptz IS NULL OR s.created_at < $2) ORDER BY s.created_at DESC LIMIT $3`,
		authorID, before, limit)
}

func (r *Repository) CanView(ctx context.Context, viewerID, storyID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM stories s WHERE s.id = $2 AND `+visibleSQL("$1")+`)`, viewerID, storyID).Scan(&ok)
	return ok, err
}

// CanViewStoryMedia lets the media module authorize a download of a story's
// image/video for someone who can currently see that story. (The author is
// already allowed by the media module as the file's owner.)
func (r *Repository) CanViewStoryMedia(ctx context.Context, viewerID, mediaID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM stories s WHERE s.media_id = $2 AND `+visibleSQL("$1")+`)`, viewerID, mediaID).Scan(&ok)
	return ok, err
}

func (r *Repository) AuthorOf(ctx context.Context, storyID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT author_id FROM stories WHERE id = $1 AND deleted_at IS NULL`, storyID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

func (r *Repository) SoftDelete(ctx context.Context, authorID, storyID string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE stories SET deleted_at = now() WHERE id = $1 AND author_id = $2 AND deleted_at IS NULL`, storyID, authorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordView is idempotent per (story, viewer). isNew tells the caller
// whether to notify the author; viewCount is the running total.
func (r *Repository) RecordView(ctx context.Context, viewerID, storyID string) (isNew bool, viewCount int64, err error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO story_views (story_id, viewer_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, storyID, viewerID)
	if err != nil {
		return false, 0, err
	}
	err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM story_views WHERE story_id = $1`, storyID).Scan(&viewCount)
	return tag.RowsAffected() == 1, viewCount, err
}

func (r *Repository) ListViewers(ctx context.Context, storyID string) ([]Viewer, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT v.viewer_id, u.username, p.display_name, v.viewed_at, rc.emoji
		FROM story_views v
		JOIN users u ON u.id = v.viewer_id
		JOIN user_profiles p ON p.user_id = v.viewer_id
		LEFT JOIN story_reactions rc ON rc.story_id = v.story_id AND rc.user_id = v.viewer_id
		WHERE v.story_id = $1
		ORDER BY v.viewed_at DESC
		LIMIT 500
	`, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Viewer
	for rows.Next() {
		var v Viewer
		if err := rows.Scan(&v.UserID, &v.Username, &v.DisplayName, &v.ViewedAt, &v.Reaction); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) SetReaction(ctx context.Context, storyID, userID, emoji string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO story_reactions (story_id, user_id, emoji) VALUES ($1, $2, $3)
		ON CONFLICT (story_id, user_id) DO UPDATE SET emoji = EXCLUDED.emoji, created_at = now()
	`, storyID, userID, emoji)
	return err
}

func (r *Repository) RemoveReaction(ctx context.Context, storyID, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM story_reactions WHERE story_id = $1 AND user_id = $2`, storyID, userID)
	return err
}

func (r *Repository) AddReply(ctx context.Context, storyID, replierID, body string) (Reply, error) {
	rep := Reply{StoryID: storyID, ReplierID: replierID, Body: body}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO story_replies (story_id, replier_id, body) VALUES ($1, $2, $3) RETURNING id, created_at`,
		storyID, replierID, body).Scan(&rep.ID, &rep.CreatedAt)
	return rep, err
}

func (r *Repository) ListReplies(ctx context.Context, storyID string) ([]Reply, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, story_id, replier_id, body, created_at FROM story_replies WHERE story_id = $1 ORDER BY created_at ASC LIMIT 500`, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reply
	for rows.Next() {
		var rep Reply
		if err := rows.Scan(&rep.ID, &rep.StoryID, &rep.ReplierID, &rep.Body, &rep.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

func (r *Repository) Mute(ctx context.Context, muterID, mutedID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO story_mutes (muter_id, muted_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, muterID, mutedID)
	return err
}

func (r *Repository) Unmute(ctx context.Context, muterID, mutedID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM story_mutes WHERE muter_id = $1 AND muted_id = $2`, muterID, mutedID)
	return err
}

// AudienceFor lists who can see a just-posted story, for the live
// "story.created" push. Candidates are limited to the author's contacts plus
// the story's include-list and mentions (never a scan of all users), then
// filtered through the same visibility rule the feed uses.
func (r *Repository) AudienceFor(ctx context.Context, storyID, authorID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT cand.id FROM (
			SELECT contact_user_id AS id FROM contacts WHERE owner_id = $2 AND contact_user_id IS NOT NULL
			UNION SELECT user_id FROM story_audience WHERE story_id = $1 AND kind = 'include'
			UNION SELECT user_id FROM story_mentions WHERE story_id = $1
		) cand
		JOIN stories s ON s.id = $1
		WHERE `+visibleSQL("cand.id"), storyID, authorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// groupFeed turns flat feed rows (ordered by author then time) into one entry
// per author. Entries with something unseen come first; within each group,
// the author whose newest story is most recent comes first. Muted authors
// are kept (flagged) rather than dropped so the client can offer "unmute".
func groupFeed(rows []feedRow) []FeedEntry {
	byAuthor := map[string]*FeedEntry{}
	var order []string
	for _, row := range rows {
		e, ok := byAuthor[row.Story.AuthorID]
		if !ok {
			e = &FeedEntry{AuthorID: row.Story.AuthorID, Username: row.Username, DisplayName: row.DisplayName, Muted: row.Muted}
			byAuthor[row.Story.AuthorID] = e
			order = append(order, row.Story.AuthorID)
		}
		if !row.Story.Seen {
			e.HasUnseen = true
		}
		e.Stories = append(e.Stories, row.Story)
	}

	out := make([]FeedEntry, 0, len(order))
	for _, id := range order {
		out = append(out, *byAuthor[id])
	}
	newest := func(e FeedEntry) time.Time { return e.Stories[len(e.Stories)-1].CreatedAt }
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].HasUnseen != out[j].HasUnseen {
			return out[i].HasUnseen
		}
		return newest(out[i]).After(newest(out[j]))
	})
	return out
}
