package communities

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("communities: not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, creatorID string, req CreateCommunityRequest) (Community, error) {
	var c Community
	var description, iconMediaID *string
	if req.Description != "" {
		description = &req.Description
	}
	if req.IconMediaID != "" {
		iconMediaID = &req.IconMediaID
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO communities (name, description, icon_media_id, discoverable, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, description, icon_media_id, invite_code, discoverable, created_by, created_at
	`, req.Name, description, iconMediaID, req.Discoverable, creatorID).Scan(
		&c.ID, &c.Name, &c.Description, &c.IconMediaID, &c.InviteCode, &c.Discoverable, &c.CreatedBy, &c.CreatedAt,
	)
	return c, err
}

func (r *Repository) Get(ctx context.Context, communityID string) (Community, error) {
	var c Community
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, icon_media_id, invite_code, discoverable, created_by, created_at
		FROM communities WHERE id = $1
	`, communityID).Scan(&c.ID, &c.Name, &c.Description, &c.IconMediaID, &c.InviteCode, &c.Discoverable, &c.CreatedBy, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Community{}, ErrNotFound
	}
	return c, err
}

// GetByInviteCode returns ErrInviteNotFound (not the generic ErrNotFound)
// so callers — and the HTTP layer — can tell "this invite link is bad"
// apart from "that community id doesn't exist".
func (r *Repository) GetByInviteCode(ctx context.Context, code string) (Community, error) {
	var c Community
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, icon_media_id, invite_code, discoverable, created_by, created_at
		FROM communities WHERE invite_code = $1
	`, code).Scan(&c.ID, &c.Name, &c.Description, &c.IconMediaID, &c.InviteCode, &c.Discoverable, &c.CreatedBy, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Community{}, ErrInviteNotFound
	}
	return c, err
}

func (r *Repository) Update(ctx context.Context, communityID string, req UpdateCommunityRequest) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE communities SET
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			icon_media_id = COALESCE($4, icon_media_id),
			discoverable = COALESCE($5, discoverable)
		WHERE id = $1
	`, communityID, req.Name, req.Description, req.IconMediaID, req.Discoverable)
	return err
}

func (r *Repository) MemberRoleOf(ctx context.Context, communityID, userID string) (role Role, isMember bool, err error) {
	var roleStr string
	err = r.pool.QueryRow(ctx,
		`SELECT role FROM community_members WHERE community_id = $1 AND user_id = $2`, communityID, userID).Scan(&roleStr)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return Role(roleStr), true, nil
}

func (r *Repository) AddMember(ctx context.Context, communityID, userID string, role Role) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO community_members (community_id, user_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		communityID, userID, string(role))
	return err
}

func (r *Repository) RemoveMember(ctx context.Context, communityID, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM community_members WHERE community_id = $1 AND user_id = $2`, communityID, userID)
	return err
}

func (r *Repository) UpdateMemberRole(ctx context.Context, communityID, userID string, role Role) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE community_members SET role = $3 WHERE community_id = $1 AND user_id = $2`, communityID, userID, string(role))
	return err
}

func (r *Repository) ListMembers(ctx context.Context, communityID string) ([]Member, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, role, joined_at FROM community_members WHERE community_id = $1 ORDER BY joined_at ASC`, communityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		var roleStr string
		if err := rows.Scan(&m.UserID, &roleStr, &m.JoinedAt); err != nil {
			return nil, err
		}
		m.Role = Role(roleStr)
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListForUser returns every community userID currently belongs to — what
// a "Communities" tab needs, as distinct from SearchDiscoverable (which
// only ever returns communities the caller might NOT be in yet).
func (r *Repository) ListForUser(ctx context.Context, userID string) ([]Community, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.name, c.description, c.icon_media_id, c.invite_code, c.discoverable, c.created_by, c.created_at
		FROM communities c
		JOIN community_members cm ON cm.community_id = c.id
		WHERE cm.user_id = $1
		ORDER BY c.name ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Community
	for rows.Next() {
		var c Community
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.IconMediaID, &c.InviteCode, &c.Discoverable, &c.CreatedBy, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) MemberCount(ctx context.Context, communityID string) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM community_members WHERE community_id = $1`, communityID).Scan(&n)
	return n, err
}

func (r *Repository) Ban(ctx context.Context, communityID, userID, bannedBy string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM community_members WHERE community_id = $1 AND user_id = $2`, communityID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO community_banned_users (community_id, user_id, banned_by) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		communityID, userID, bannedBy,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) Unban(ctx context.Context, communityID, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM community_banned_users WHERE community_id = $1 AND user_id = $2`, communityID, userID)
	return err
}

func (r *Repository) IsBanned(ctx context.Context, communityID, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM community_banned_users WHERE community_id = $1 AND user_id = $2)`, communityID, userID).Scan(&exists)
	return exists, err
}

// --- Groups within a community ---

func (r *Repository) LinkGroup(ctx context.Context, communityID, conversationID string, isAnnouncement bool) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO community_groups (community_id, conversation_id, is_announcement) VALUES ($1, $2, $3)`,
		communityID, conversationID, isAnnouncement)
	return err
}

func (r *Repository) UnlinkGroup(ctx context.Context, communityID, conversationID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM community_groups WHERE community_id = $1 AND conversation_id = $2 AND NOT is_announcement`,
		communityID, conversationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound // either not linked, or it's the (protected) announcement group
	}
	return nil
}

func (r *Repository) AnnouncementConversationID(ctx context.Context, communityID string) (string, bool, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT conversation_id FROM community_groups WHERE community_id = $1 AND is_announcement`, communityID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

func (r *Repository) ListGroups(ctx context.Context, communityID string) ([]GroupRef, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT cg.conversation_id, g.name, cg.is_announcement
		FROM community_groups cg
		JOIN groups g ON g.conversation_id = cg.conversation_id
		WHERE cg.community_id = $1
		ORDER BY cg.is_announcement DESC, cg.added_at ASC
	`, communityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GroupRef
	for rows.Next() {
		var g GroupRef
		if err := rows.Scan(&g.ConversationID, &g.Name, &g.IsAnnouncement); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// --- Invite links ---

func generateInviteCode() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(b), "=")), nil
}

func (r *Repository) SetNewInviteCode(ctx context.Context, communityID string) (string, error) {
	code, err := generateInviteCode()
	if err != nil {
		return "", err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE communities SET invite_code = $2 WHERE id = $1`, communityID, code); err != nil {
		return "", err
	}
	return code, nil
}

func (r *Repository) RevokeInviteCode(ctx context.Context, communityID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE communities SET invite_code = NULL WHERE id = $1`, communityID)
	return err
}

// --- Search & reports ---

func (r *Repository) SearchDiscoverable(ctx context.Context, prefix string, limit int) ([]Community, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description, icon_media_id, invite_code, discoverable, created_by, created_at
		FROM communities WHERE discoverable AND name ILIKE $1 || '%'
		ORDER BY name ASC LIMIT $2
	`, prefix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Community
	for rows.Next() {
		var c Community
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.IconMediaID, &c.InviteCode, &c.Discoverable, &c.CreatedBy, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) Report(ctx context.Context, reporterID, communityID, reason string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO community_reports (reporter_id, community_id, reason) VALUES ($1, $2, $3)`, reporterID, communityID, reason)
	return err
}
