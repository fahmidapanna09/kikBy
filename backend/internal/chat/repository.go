package chat

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound       = errors.New("chat: not found")
	ErrNotAMember     = errors.New("chat: user is not a member of this conversation")
	ErrDuplicateSend  = errors.New("chat: duplicate client_tmp_id") // not currently returned as an error path; see InsertMessage doc
	ErrSendNotAllowed = errors.New("chat: this group only allows admins to send messages")
	ErrBlocked        = errors.New("chat: messaging is not allowed between these users")
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// directKey produces a canonical, order-independent key for a 1:1
// conversation between two users so GetOrCreateDirect can upsert instead of
// racing two concurrent inserts into two different rows for the same pair.
func directKey(userA, userB string) string {
	ids := []string{userA, userB}
	sort.Strings(ids)
	return strings.Join(ids, ":")
}

func (r *Repository) GetOrCreateDirectConversation(ctx context.Context, userA, userB string) (Conversation, error) {
	key := directKey(userA, userB)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Conversation{}, err
	}
	defer tx.Rollback(ctx)

	var c Conversation
	err = tx.QueryRow(ctx, `SELECT id, type, last_message_at FROM conversations WHERE direct_key = $1`, key).
		Scan(&c.ID, &c.Type, &c.LastMessageAt)
	if err == nil {
		return c, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, err
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO conversations (type, direct_key) VALUES ('direct', $1)
		 ON CONFLICT (direct_key) DO UPDATE SET direct_key = EXCLUDED.direct_key
		 RETURNING id, type, last_message_at`,
		key,
	).Scan(&c.ID, &c.Type, &c.LastMessageAt)
	if err != nil {
		return Conversation{}, err
	}

	for _, uid := range []string{userA, userB} {
		if _, err := tx.Exec(ctx,
			`INSERT INTO conversation_members (conversation_id, user_id) VALUES ($1, $2)
			 ON CONFLICT (conversation_id, user_id) DO NOTHING`,
			c.ID, uid,
		); err != nil {
			return Conversation{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Conversation{}, err
	}
	return c, nil
}

func (r *Repository) IsMember(ctx context.Context, conversationID, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id = $1 AND user_id = $2 AND left_at IS NULL)`,
		conversationID, userID,
	).Scan(&exists)
	return exists, err
}

// GetConversationType is used by the blocking check in service.go — only
// direct (1:1) conversations are subject to block enforcement; group
// messages aren't blocked by a pairwise relationship the same way.
func (r *Repository) GetConversationType(ctx context.Context, conversationID string) (string, error) {
	var t string
	err := r.pool.QueryRow(ctx, `SELECT type FROM conversations WHERE id = $1`, conversationID).Scan(&t)
	return t, err
}

// MemberRoleOf returns the caller's role in a conversation ("member" for
// every participant of a direct/1:1 conversation, since roles only carry
// meaning for groups) and whether they're a member at all.
func (r *Repository) MemberRoleOf(ctx context.Context, conversationID, userID string) (role MemberRole, isMember bool, err error) {
	var roleStr string
	err = r.pool.QueryRow(ctx,
		`SELECT role FROM conversation_members WHERE conversation_id = $1 AND user_id = $2 AND left_at IS NULL`,
		conversationID, userID,
	).Scan(&roleStr)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return MemberRole(roleStr), true, nil
}

// CanUserSend enforces a group's who_can_send setting. Direct (1:1)
// conversations have no groups row and are always sendable by any member —
// the LEFT JOIN + COALESCE makes that the default when g.who_can_send is NULL.
func (r *Repository) CanUserSend(ctx context.Context, conversationID string, role MemberRole) (bool, error) {
	var whoCanSend string
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(g.who_can_send, 'everyone')
		FROM conversations c
		LEFT JOIN groups g ON g.conversation_id = c.id
		WHERE c.id = $1
	`, conversationID).Scan(&whoCanSend)
	if err != nil {
		return false, err
	}
	if whoCanSend == string(PermissionEveryone) {
		return true, nil
	}
	return role == RoleOwner || role == RoleAdmin, nil
}

func (r *Repository) OtherMembers(ctx context.Context, conversationID, excludingUserID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id FROM conversation_members WHERE conversation_id = $1 AND user_id <> $2 AND left_at IS NULL`,
		conversationID, excludingUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}

// InsertMessage assigns the next per-conversation seq under a row lock on
// the conversation (SELECT ... FOR UPDATE) so concurrent sends from
// different devices never collide on seq, then inserts the message. It is
// idempotent on (conversation_id, sender_id, client_tmp_id): a retried send
// with the same client_tmp_id returns the original row instead of erroring
// or duplicating, which is what lets the client safely retry on timeout.
func (r *Repository) InsertMessage(ctx context.Context, conversationID, senderID string, req SendMessageRequest) (Message, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, false, err
	}
	defer tx.Rollback(ctx)

	var existing Message
	err = tx.QueryRow(ctx, `
		SELECT id, conversation_id, seq, sender_id, client_tmp_id, type, body, reply_to_id, created_at, edited_at, forwarded_from_message_id
		FROM messages WHERE conversation_id = $1 AND sender_id = $2 AND client_tmp_id = $3
	`, conversationID, senderID, req.ClientTmpID).Scan(
		&existing.ID, &existing.ConversationID, &existing.Seq, &existing.SenderID, &existing.ClientTmpID,
		&existing.Type, &existing.Body, &existing.ReplyToID, &existing.CreatedAt, &existing.EditedAt, &existing.ForwardedFromMessageID,
	)
	if err == nil {
		return existing, false, tx.Commit(ctx) // was already inserted, not a new send
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Message{}, false, err
	}

	if _, err := tx.Exec(ctx, `SELECT id FROM conversations WHERE id = $1 FOR UPDATE`, conversationID); err != nil {
		return Message{}, false, err
	}

	var nextSeq int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(seq), 0) + 1 FROM messages WHERE conversation_id = $1`, conversationID).Scan(&nextSeq); err != nil {
		return Message{}, false, err
	}

	var body *string
	if req.Body != "" {
		body = &req.Body
	}
	var replyTo *string
	if req.ReplyToID != "" {
		replyTo = &req.ReplyToID
	}
	var forwardedFrom *string
	if req.ForwardedFromMessageID != "" {
		forwardedFrom = &req.ForwardedFromMessageID
	}

	var m Message
	err = tx.QueryRow(ctx, `
		INSERT INTO messages (conversation_id, seq, sender_id, client_tmp_id, type, body, reply_to_id, forwarded_from_message_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, conversation_id, seq, sender_id, client_tmp_id, type, body, reply_to_id, created_at, edited_at, forwarded_from_message_id
	`, conversationID, nextSeq, senderID, req.ClientTmpID, req.Type, body, replyTo, forwardedFrom).Scan(
		&m.ID, &m.ConversationID, &m.Seq, &m.SenderID, &m.ClientTmpID,
		&m.Type, &m.Body, &m.ReplyToID, &m.CreatedAt, &m.EditedAt, &m.ForwardedFromMessageID,
	)
	if err != nil {
		return Message{}, false, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE conversations SET last_message_id = $1, last_message_at = $2 WHERE id = $3`,
		m.ID, m.CreatedAt, conversationID,
	); err != nil {
		return Message{}, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Message{}, false, err
	}
	return m, true, nil
}

// ListMessages returns messages older than the cursor (a seq number; 0 =
// start from the newest), newest-first, for infinite-scroll pagination.
// Excludes messages the requesting user has deleted-for-me.
func (r *Repository) ListMessages(ctx context.Context, conversationID, requestingUserID string, beforeSeq int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `
		SELECT m.id, m.conversation_id, m.seq, m.sender_id, m.client_tmp_id, m.type, m.body, m.reply_to_id, m.created_at, m.edited_at, m.forwarded_from_message_id, m.deleted_for_everyone_at
		FROM messages m
		WHERE m.conversation_id = $1
		  AND ($2 = 0 OR m.seq < $2)
		  AND NOT EXISTS (SELECT 1 FROM message_deletions d WHERE d.message_id = m.id AND d.user_id = $3)
		ORDER BY m.seq DESC
		LIMIT $4
	`
	rows, err := r.pool.Query(ctx, query, conversationID, beforeSeq, requestingUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Seq, &m.SenderID, &m.ClientTmpID, &m.Type, &m.Body, &m.ReplyToID, &m.CreatedAt, &m.EditedAt, &m.ForwardedFromMessageID, &m.DeletedAt); err != nil {
			return nil, err
		}
		if m.DeletedAt != nil {
			m.Body = nil // tombstone: never ship the content of a deleted-for-everyone message
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.attachReactions(ctx, out); err != nil {
		return nil, err
	}
	if err := r.attachAttachments(ctx, out); err != nil {
		return nil, err
	}
	if err := r.attachStatus(ctx, out, requestingUserID); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) MarkDelivered(ctx context.Context, messageID, userID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO message_deliveries (message_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		messageID, userID,
	)
	return err
}

func (r *Repository) MarkRead(ctx context.Context, conversationID, messageID, userID string, seq int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO message_reads (message_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		messageID, userID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE conversation_members SET last_read_seq = GREATEST(last_read_seq, $1) WHERE conversation_id = $2 AND user_id = $3`,
		seq, conversationID, userID,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListConversationsForUser returns the user's conversations with last
// message + unread count, newest activity first — the data the "Chats" tab
// needs in one query rather than N+1 round trips.
func (r *Repository) ListConversationsForUser(ctx context.Context, userID string, limit int) ([]ConversationSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.type,
		       CASE WHEN c.type = 'direct'
		            THEN (SELECT user_id FROM conversation_members WHERE conversation_id = c.id AND user_id <> $1 LIMIT 1)
		       END AS peer_id,
		       g.name AS group_name,
		       lm.id, lm.seq, lm.sender_id, lm.client_tmp_id, lm.type,
		       CASE WHEN lm.deleted_for_everyone_at IS NULL THEN lm.body END, lm.created_at,
		       GREATEST(0, (SELECT COUNT(*) FROM messages mm WHERE mm.conversation_id = c.id AND mm.seq > cm.last_read_seq AND mm.sender_id <> $1 AND mm.deleted_for_everyone_at IS NULL)) AS unread,
		       cm.pinned_at, cm.archived_at, cm.muted_until
		FROM conversations c
		JOIN conversation_members cm ON cm.conversation_id = c.id AND cm.user_id = $1
		LEFT JOIN groups g ON g.conversation_id = c.id
		LEFT JOIN messages lm ON lm.id = c.last_message_id
		WHERE cm.left_at IS NULL
		ORDER BY (cm.pinned_at IS NULL), cm.pinned_at DESC, c.last_message_at DESC NULLS LAST
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ConversationSummary
	for rows.Next() {
		var cs ConversationSummary
		var peerID *string
		var lmID, lmSenderID, lmTmpID, lmType, lmBody *string
		var lmSeq *int64
		var lmCreatedAt *time.Time
		if err := rows.Scan(&cs.ID, &cs.Type, &peerID, &cs.Name, &lmID, &lmSeq, &lmSenderID, &lmTmpID, &lmType, &lmBody, &lmCreatedAt, &cs.UnreadCount, &cs.PinnedAt, &cs.ArchivedAt, &cs.MutedUntil); err != nil {
			return nil, err
		}
		if peerID != nil {
			cs.PeerUserID = *peerID
		}
		if lmID != nil {
			cs.LastMessage = &Message{
				ID: *lmID, ConversationID: cs.ID, Seq: *lmSeq, SenderID: *lmSenderID,
				ClientTmpID: *lmTmpID, Type: *lmType, Body: lmBody, CreatedAt: *lmCreatedAt,
			}
		}
		out = append(out, cs)
	}
	return out, rows.Err()
}

func (r *Repository) errNotAMember() error { return fmt.Errorf("%w", ErrNotAMember) }

// ListAttachmentMediaIDs returns the media_files.id values attached to a
// message, in display order. message_attachments is defined in the Phase 3
// media migration (it references media_files), but conceptually it's an
// edge on `messages`, so chat reads it directly here rather than round
// tripping through the media module for something this simple.
func (r *Repository) ListAttachmentMediaIDs(ctx context.Context, messageID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT media_id FROM message_attachments WHERE message_id = $1 ORDER BY position ASC`, messageID)
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

// ListAttachmentInfo returns display metadata for a message's attachments in
// display order.
func (r *Repository) ListAttachmentInfo(ctx context.Context, messageID string) ([]AttachmentInfo, error) {
	msgs := []Message{{ID: messageID}}
	if err := r.attachAttachments(ctx, msgs); err != nil {
		return nil, err
	}
	return msgs[0].Attachments, nil
}

// DisplayNameOf is the name shown in push notifications: the profile name, else the username ("" if unknown).
func (r *Repository) DisplayNameOf(ctx context.Context, userID string) string {
	var name string
	_ = r.pool.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(p.display_name, ''), u.username, '')
		FROM users u LEFT JOIN user_profiles p ON p.user_id = u.id
		WHERE u.id = $1`, userID).Scan(&name)
	return name
}
