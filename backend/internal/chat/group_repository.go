package chat

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrGroupNotFound    = errors.New("chat: group not found")
	ErrInviteNotFound   = errors.New("chat: invite code not found or revoked")
	ErrPermissionDenied = errors.New("chat: insufficient permission for this action")
)

// CreateGroup creates the underlying conversation row plus its groups
// metadata row plus conversation_members rows, all in one transaction —
// a group is never left half-created.
func (r *Repository) CreateGroup(ctx context.Context, creatorID string, req CreateGroupRequest) (Group, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Group{}, err
	}
	defer tx.Rollback(ctx)

	var conversationID string
	if err := tx.QueryRow(ctx, `INSERT INTO conversations (type) VALUES ('group') RETURNING id`).Scan(&conversationID); err != nil {
		return Group{}, err
	}

	var description *string
	if req.Description != "" {
		description = &req.Description
	}
	var iconMediaID *string
	if req.IconMediaID != "" {
		iconMediaID = &req.IconMediaID
	}

	var g Group
	err = tx.QueryRow(ctx, `
		INSERT INTO groups (conversation_id, name, description, icon_media_id, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING conversation_id, name, description, icon_media_id, who_can_send, who_can_add_members, who_can_edit_info, invite_code, created_by, created_at
	`, conversationID, req.Name, description, iconMediaID, creatorID).Scan(
		&g.ConversationID, &g.Name, &g.Description, &g.IconMediaID, &g.WhoCanSend, &g.WhoCanAddMembers, &g.WhoCanEditInfo, &g.InviteCode, &g.CreatedBy, &g.CreatedAt,
	)
	if err != nil {
		return Group{}, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO conversation_members (conversation_id, user_id, role) VALUES ($1, $2, 'owner')`,
		conversationID, creatorID,
	); err != nil {
		return Group{}, err
	}

	for _, uid := range req.MemberUserIDs {
		if uid == creatorID {
			continue // already added as owner above
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO conversation_members (conversation_id, user_id, role) VALUES ($1, $2, 'member') ON CONFLICT (conversation_id, user_id) DO NOTHING`,
			conversationID, uid,
		); err != nil {
			return Group{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Group{}, err
	}
	return g, nil
}

func (r *Repository) GetGroup(ctx context.Context, conversationID string) (Group, error) {
	var g Group
	err := r.pool.QueryRow(ctx, `
		SELECT conversation_id, name, description, icon_media_id, who_can_send, who_can_add_members, who_can_edit_info, invite_code, created_by, created_at
		FROM groups WHERE conversation_id = $1
	`, conversationID).Scan(
		&g.ConversationID, &g.Name, &g.Description, &g.IconMediaID, &g.WhoCanSend, &g.WhoCanAddMembers, &g.WhoCanEditInfo, &g.InviteCode, &g.CreatedBy, &g.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrGroupNotFound
	}
	return g, err
}

func (r *Repository) UpdateGroup(ctx context.Context, conversationID string, req UpdateGroupRequest) error {
	// Built as a single dynamic-ish statement using COALESCE so a partial
	// update (only `name`, say) leaves the other fields untouched, without
	// needing a hand-rolled query builder for three optional fields.
	_, err := r.pool.Exec(ctx, `
		UPDATE groups SET
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			icon_media_id = COALESCE($4, icon_media_id)
		WHERE conversation_id = $1
	`, conversationID, req.Name, req.Description, req.IconMediaID)
	return err
}

func (r *Repository) AddMembers(ctx context.Context, conversationID string, userIDs []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, uid := range userIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO conversation_members (conversation_id, user_id, role)
			VALUES ($1, $2, 'member')
			ON CONFLICT (conversation_id, user_id) DO UPDATE SET left_at = NULL
		`, conversationID, uid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) RemoveMember(ctx context.Context, conversationID, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE conversation_members SET left_at = now() WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	)
	return err
}

func (r *Repository) UpdateMemberRole(ctx context.Context, conversationID, userID string, role MemberRole) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE conversation_members SET role = $3 WHERE conversation_id = $1 AND user_id = $2 AND left_at IS NULL`,
		conversationID, userID, string(role),
	)
	return err
}

func (r *Repository) ListMembers(ctx context.Context, conversationID string) ([]Member, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, role, joined_at FROM conversation_members WHERE conversation_id = $1 AND left_at IS NULL ORDER BY joined_at ASC`,
		conversationID,
	)
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
		m.Role = MemberRole(roleStr)
		out = append(out, m)
	}
	return out, rows.Err()
}

// MemberRoleOfString is MemberRoleOf with the role returned as a plain
// string — for callers outside this package (e.g. communities.Service's
// GroupMembershipChecker) that can't reference the unexported-outside-chat
// MemberRole type directly. Go's structural typing requires an exact
// return-type match, so a method returning MemberRole does NOT satisfy an
// interface declared with a plain string return, even though MemberRole's
// underlying type is string — hence this adapter.
func (r *Repository) MemberRoleOfString(ctx context.Context, conversationID, userID string) (string, bool, error) {
	role, isMember, err := r.MemberRoleOf(ctx, conversationID, userID)
	return string(role), isMember, err
}

// generateInviteCode produces a short, URL-safe, unguessable code —
// base32 (Crockford-ish alphabet via encoding/base32 lowercased) of 10
// random bytes, trimmed of padding.
func generateInviteCode() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(b), "="))
	return code, nil
}

// SetWhoCanSend is used at community-creation time to make the community's
// announcement group admins-only immediately, without a separate "update
// permissions" API existing yet for groups in general (see UpdateGroup,
// which only covers name/description/icon).
func (r *Repository) SetWhoCanSend(ctx context.Context, conversationID string, scope PermissionScope) error {
	_, err := r.pool.Exec(ctx, `UPDATE groups SET who_can_send = $2 WHERE conversation_id = $1`, conversationID, string(scope))
	return err
}

func (r *Repository) SetNewInviteCode(ctx context.Context, conversationID string) (string, error) {
	code, err := generateInviteCode()
	if err != nil {
		return "", err
	}
	_, err = r.pool.Exec(ctx, `UPDATE groups SET invite_code = $2 WHERE conversation_id = $1`, conversationID, code)
	if err != nil {
		return "", err
	}
	return code, nil
}

func (r *Repository) RevokeInviteCode(ctx context.Context, conversationID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE groups SET invite_code = NULL WHERE conversation_id = $1`, conversationID)
	return err
}

func (r *Repository) GetGroupByInviteCode(ctx context.Context, code string) (Group, error) {
	var g Group
	err := r.pool.QueryRow(ctx, `
		SELECT conversation_id, name, description, icon_media_id, who_can_send, who_can_add_members, who_can_edit_info, invite_code, created_by, created_at
		FROM groups WHERE invite_code = $1
	`, code).Scan(
		&g.ConversationID, &g.Name, &g.Description, &g.IconMediaID, &g.WhoCanSend, &g.WhoCanAddMembers, &g.WhoCanEditInfo, &g.InviteCode, &g.CreatedBy, &g.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrInviteNotFound
	}
	return g, err
}

func (r *Repository) SetMuteUntil(ctx context.Context, conversationID, userID string, until *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE conversation_members SET muted_until = $3 WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID, until,
	)
	return err
}
