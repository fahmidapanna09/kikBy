package channels

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("channels: not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (r *Repository) Create(ctx context.Context, creatorID string, req CreateChannelRequest) (Channel, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Channel{}, err
	}
	defer tx.Rollback(ctx)

	var c Channel
	err = tx.QueryRow(ctx, `
		INSERT INTO channels (name, description, icon_media_id, category, is_public, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, description, icon_media_id, category, is_public, invite_code, created_by, created_at
	`, req.Name, nullable(req.Description), nullable(req.IconMediaID), nullable(req.Category), req.IsPublic, creatorID).Scan(
		&c.ID, &c.Name, &c.Description, &c.IconMediaID, &c.Category, &c.IsPublic, &c.InviteCode, &c.CreatedBy, &c.CreatedAt,
	)
	if err != nil {
		return Channel{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO channel_admins (channel_id, user_id, role) VALUES ($1, $2, 'owner')`, c.ID, creatorID,
	); err != nil {
		return Channel{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Channel{}, err
	}
	return c, nil
}

func (r *Repository) Get(ctx context.Context, channelID string) (Channel, error) {
	var c Channel
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, icon_media_id, category, is_public, invite_code, created_by, created_at
		FROM channels WHERE id = $1 AND deleted_at IS NULL
	`, channelID).Scan(&c.ID, &c.Name, &c.Description, &c.IconMediaID, &c.Category, &c.IsPublic, &c.InviteCode, &c.CreatedBy, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	return c, err
}

func (r *Repository) GetByInviteCode(ctx context.Context, code string) (Channel, error) {
	var c Channel
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, icon_media_id, category, is_public, invite_code, created_by, created_at
		FROM channels WHERE invite_code = $1 AND deleted_at IS NULL
	`, code).Scan(&c.ID, &c.Name, &c.Description, &c.IconMediaID, &c.Category, &c.IsPublic, &c.InviteCode, &c.CreatedBy, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	return c, err
}

func (r *Repository) Update(ctx context.Context, channelID string, req UpdateChannelRequest) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE channels SET
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			icon_media_id = COALESCE($4, icon_media_id),
			category = COALESCE($5, category),
			is_public = COALESCE($6, is_public)
		WHERE id = $1
	`, channelID, req.Name, req.Description, req.IconMediaID, req.Category, req.IsPublic)
	return err
}

func (r *Repository) SoftDelete(ctx context.Context, channelID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE channels SET deleted_at = now() WHERE id = $1`, channelID)
	return err
}

// --- Admins ---

func (r *Repository) AdminRole(ctx context.Context, channelID, userID string) (role string, isAdmin bool, err error) {
	err = r.pool.QueryRow(ctx, `SELECT role FROM channel_admins WHERE channel_id = $1 AND user_id = $2`, channelID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return role, err == nil, err
}

func (r *Repository) AddAdmin(ctx context.Context, channelID, userID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO channel_admins (channel_id, user_id, role) VALUES ($1, $2, 'admin') ON CONFLICT DO NOTHING`, channelID, userID)
	return err
}

func (r *Repository) RemoveAdmin(ctx context.Context, channelID, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM channel_admins WHERE channel_id = $1 AND user_id = $2 AND role <> 'owner'`, channelID, userID)
	return err
}

func (r *Repository) ListAdmins(ctx context.Context, channelID string) ([]Admin, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, role, added_at FROM channel_admins WHERE channel_id = $1 ORDER BY added_at ASC`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Admin
	for rows.Next() {
		var a Admin
		if err := rows.Scan(&a.UserID, &a.Role, &a.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- Followers ---

func (r *Repository) Follow(ctx context.Context, channelID, userID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO channel_followers (channel_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, channelID, userID)
	return err
}

func (r *Repository) Unfollow(ctx context.Context, channelID, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM channel_followers WHERE channel_id = $1 AND user_id = $2`, channelID, userID)
	return err
}

func (r *Repository) IsFollowing(ctx context.Context, channelID, userID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM channel_followers WHERE channel_id = $1 AND user_id = $2)`, channelID, userID).Scan(&ok)
	return ok, err
}

func (r *Repository) FollowerCount(ctx context.Context, channelID string) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM channel_followers WHERE channel_id = $1`, channelID).Scan(&n)
	return n, err
}

func (r *Repository) ListFollowerIDs(ctx context.Context, channelID string, mutedOnly, unmutedOnly bool) ([]string, error) {
	query := `SELECT user_id FROM channel_followers WHERE channel_id = $1`
	if unmutedOnly {
		query += ` AND NOT muted`
	}
	if mutedOnly {
		query += ` AND muted`
	}
	rows, err := r.pool.Query(ctx, query, channelID)
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

func (r *Repository) SetFollowMuted(ctx context.Context, channelID, userID string, muted bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE channel_followers SET muted = $3 WHERE channel_id = $1 AND user_id = $2`, channelID, userID, muted)
	return err
}

// --- Posts ---

const postCols = `p.id, p.channel_id, p.author_id, p.seq, p.post_type, p.body, p.media_id, p.link_url, p.created_at, p.edited_at`

func scanPost(row pgx.Row, p *Post) error {
	return row.Scan(&p.ID, &p.ChannelID, &p.AuthorID, &p.Seq, &p.Type, &p.Body, &p.MediaID, &p.LinkURL, &p.CreatedAt, &p.EditedAt)
}

func (r *Repository) CreatePost(ctx context.Context, channelID, authorID string, req CreatePostRequest) (Post, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Post{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT id FROM channels WHERE id = $1 FOR UPDATE`, channelID); err != nil {
		return Post{}, err
	}
	var nextSeq int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(seq), 0) + 1 FROM channel_posts WHERE channel_id = $1`, channelID).Scan(&nextSeq); err != nil {
		return Post{}, err
	}

	var p Post
	err = tx.QueryRow(ctx, `
		INSERT INTO channel_posts (channel_id, author_id, seq, post_type, body, media_id, link_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+postCols,
		channelID, authorID, nextSeq, req.Type, nullable(req.Body), nullable(req.MediaID), nullable(req.LinkURL),
	).Scan(&p.ID, &p.ChannelID, &p.AuthorID, &p.Seq, &p.Type, &p.Body, &p.MediaID, &p.LinkURL, &p.CreatedAt, &p.EditedAt)
	if err != nil {
		return Post{}, err
	}

	if req.Type == PostPoll {
		for i, optText := range req.PollOptions {
			if _, err := tx.Exec(ctx,
				`INSERT INTO channel_poll_options (post_id, text, position) VALUES ($1, $2, $3)`, p.ID, optText, i,
			); err != nil {
				return Post{}, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Post{}, err
	}
	return p, nil
}

func (r *Repository) GetPost(ctx context.Context, postID string) (Post, error) {
	var p Post
	err := scanPost(r.pool.QueryRow(ctx, `SELECT `+postCols+` FROM channel_posts p WHERE p.id = $1 AND p.deleted_at IS NULL`, postID), &p)
	if errors.Is(err, pgx.ErrNoRows) {
		return Post{}, ErrNotFound
	}
	return p, err
}

func (r *Repository) EditPost(ctx context.Context, postID, body string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE channel_posts SET body = $2, edited_at = now() WHERE id = $1 AND deleted_at IS NULL`, postID, body)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) DeletePost(ctx context.Context, postID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE channel_posts SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, postID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPosts pages newest-first by seq; beforeSeq=0 starts at the newest.
func (r *Repository) ListPosts(ctx context.Context, channelID string, beforeSeq int64, limit int) ([]Post, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+postCols+`
		FROM channel_posts p
		WHERE p.channel_id = $1 AND p.deleted_at IS NULL AND ($2 = 0 OR p.seq < $2)
		ORDER BY p.seq DESC LIMIT $3
	`, channelID, beforeSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Post
	for rows.Next() {
		var p Post
		if err := scanPost(rows, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) ChannelOfPost(ctx context.Context, postID string) (string, error) {
	var channelID string
	err := r.pool.QueryRow(ctx, `SELECT channel_id FROM channel_posts WHERE id = $1`, postID).Scan(&channelID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return channelID, err
}

// --- Views & reactions ---

func (r *Repository) RecordView(ctx context.Context, postID, userID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO channel_post_views (post_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, postID, userID)
	return err
}

func (r *Repository) ViewCount(ctx context.Context, postID string) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM channel_post_views WHERE post_id = $1`, postID).Scan(&n)
	return n, err
}

func (r *Repository) SetReaction(ctx context.Context, postID, userID, emoji string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO channel_post_reactions (post_id, user_id, emoji) VALUES ($1, $2, $3)
		ON CONFLICT (post_id, user_id) DO UPDATE SET emoji = EXCLUDED.emoji, created_at = now()
	`, postID, userID, emoji)
	return err
}

func (r *Repository) RemoveReaction(ctx context.Context, postID, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM channel_post_reactions WHERE post_id = $1 AND user_id = $2`, postID, userID)
	return err
}

func (r *Repository) ReactionSummaries(ctx context.Context, postID string) ([]ReactionSummary, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT emoji, COUNT(*) FROM channel_post_reactions WHERE post_id = $1 GROUP BY emoji ORDER BY COUNT(*) DESC`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReactionSummary
	for rows.Next() {
		var rs ReactionSummary
		if err := rows.Scan(&rs.Emoji, &rs.Count); err != nil {
			return nil, err
		}
		out = append(out, rs)
	}
	return out, rows.Err()
}

// --- Polls ---

func (r *Repository) PollOptions(ctx context.Context, postID, viewerID string) ([]PollOption, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT o.id, o.text,
		       (SELECT COUNT(*) FROM channel_poll_votes v WHERE v.option_id = o.id),
		       EXISTS(SELECT 1 FROM channel_poll_votes v WHERE v.option_id = o.id AND v.user_id = $2)
		FROM channel_poll_options o
		WHERE o.post_id = $1
		ORDER BY o.position ASC
	`, postID, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PollOption
	for rows.Next() {
		var opt PollOption
		if err := rows.Scan(&opt.ID, &opt.Text, &opt.Votes, &opt.VotedByMe); err != nil {
			return nil, err
		}
		out = append(out, opt)
	}
	return out, rows.Err()
}

// Vote enforces single-choice: any of the voter's existing votes on OTHER
// options of the same poll are cleared first (multi-select polls are a
// documented Phase 8 scope cut — see README).
func (r *Repository) Vote(ctx context.Context, postID, optionID, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM channel_poll_votes WHERE user_id = $1 AND option_id IN (SELECT id FROM channel_poll_options WHERE post_id = $2)
	`, userID, postID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO channel_poll_votes (option_id, user_id) VALUES ($1, $2)`, optionID, userID,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) OptionBelongsToPost(ctx context.Context, postID, optionID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM channel_poll_options WHERE id = $1 AND post_id = $2)`, optionID, postID).Scan(&ok)
	return ok, err
}

// --- Invite links ---

func generateInviteCode() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(b), "=")), nil
}

func (r *Repository) SetNewInviteCode(ctx context.Context, channelID string) (string, error) {
	code, err := generateInviteCode()
	if err != nil {
		return "", err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE channels SET invite_code = $2 WHERE id = $1`, channelID, code); err != nil {
		return "", err
	}
	return code, nil
}

func (r *Repository) RevokeInviteCode(ctx context.Context, channelID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE channels SET invite_code = NULL WHERE id = $1`, channelID)
	return err
}

// --- Search & reports ---

func (r *Repository) SearchPublic(ctx context.Context, query, category string, limit int) ([]Channel, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description, icon_media_id, category, is_public, invite_code, created_by, created_at
		FROM channels
		WHERE is_public AND deleted_at IS NULL
		  AND ($1 = '' OR name ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR category = $2)
		ORDER BY name ASC LIMIT $3
	`, query, category, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.IconMediaID, &c.Category, &c.IsPublic, &c.InviteCode, &c.CreatedBy, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) ListFollowed(ctx context.Context, userID string) ([]Channel, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.name, c.description, c.icon_media_id, c.category, c.is_public, c.invite_code, c.created_by, c.created_at
		FROM channels c
		JOIN channel_followers f ON f.channel_id = c.id
		WHERE f.user_id = $1 AND c.deleted_at IS NULL
		ORDER BY c.name ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.IconMediaID, &c.Category, &c.IsPublic, &c.InviteCode, &c.CreatedBy, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) Report(ctx context.Context, reporterID, channelID, reason string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO channel_reports (reporter_id, channel_id, reason) VALUES ($1, $2, $3)`, reporterID, channelID, reason)
	return err
}

// --- Phase 11 admin surface. See internal/auth/admin_repository.go's doc
// comment for why these return plain rows (scanned into the caller's own
// shape via ListReportsAdmin in service.go) rather than a shared struct.

type ReportRow struct {
	ID             string
	ReporterID     string
	ChannelID      string
	ChannelName    string
	Reason         string
	CreatedAt      time.Time
	ResolvedAt     *time.Time
	ResolvedBy     *string
	ResolutionNote *string
}

func (r *Repository) ListReports(ctx context.Context, onlyUnresolved bool, limit, offset int) ([]ReportRow, int, error) {
	query := `
		SELECT cr.id, cr.reporter_id, cr.channel_id, c.name, cr.reason, cr.created_at,
		       cr.resolved_at, cr.resolved_by, cr.resolution_note, count(*) OVER() AS total
		FROM channel_reports cr
		JOIN channels c ON c.id = cr.channel_id
	`
	if onlyUnresolved {
		query += " WHERE cr.resolved_at IS NULL"
	}
	query += " ORDER BY cr.created_at DESC LIMIT $1 OFFSET $2"

	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []ReportRow
	total := 0
	for rows.Next() {
		var rr ReportRow
		if err := rows.Scan(&rr.ID, &rr.ReporterID, &rr.ChannelID, &rr.ChannelName, &rr.Reason, &rr.CreatedAt,
			&rr.ResolvedAt, &rr.ResolvedBy, &rr.ResolutionNote, &total); err != nil {
			return nil, 0, err
		}
		out = append(out, rr)
	}
	return out, total, rows.Err()
}

func (r *Repository) ResolveReport(ctx context.Context, reportID, resolvedBy, note string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE channel_reports SET resolved_at = now(), resolved_by = $1, resolution_note = $2
		WHERE id = $3 AND resolved_at IS NULL
	`, resolvedBy, note, reportID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
