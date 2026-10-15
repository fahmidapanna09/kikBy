package chat

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNotSender      = errors.New("chat: only the sender can do that")
	ErrMessageDeleted = errors.New("chat: message was deleted")
	ErrNotEditable    = errors.New("chat: only text messages can be edited")
)

// MessageMeta is the minimal row needed to authorize an edit/delete.
type MessageMeta struct {
	ConversationID string
	SenderID       string
	Type           string
	Seq            int64
	Deleted        bool
}

func (r *Repository) GetMessageMeta(ctx context.Context, messageID string) (MessageMeta, error) {
	var m MessageMeta
	var deletedAtSet bool
	err := r.pool.QueryRow(ctx,
		`SELECT conversation_id, sender_id, type, seq, deleted_for_everyone_at IS NOT NULL FROM messages WHERE id = $1`,
		messageID,
	).Scan(&m.ConversationID, &m.SenderID, &m.Type, &m.Seq, &deletedAtSet)
	if errors.Is(err, pgx.ErrNoRows) {
		return MessageMeta{}, ErrNotFound
	}
	m.Deleted = deletedAtSet
	return m, err
}

// EditMessageBody updates the text of a live message and stamps edited_at.
func (r *Repository) EditMessageBody(ctx context.Context, messageID, body string) (Message, error) {
	var m Message
	err := r.pool.QueryRow(ctx, `
		UPDATE messages SET body = $2, edited_at = now()
		WHERE id = $1 AND deleted_for_everyone_at IS NULL
		RETURNING id, conversation_id, seq, sender_id, client_tmp_id, type, body, reply_to_id, created_at, edited_at, forwarded_from_message_id
	`, messageID, body).Scan(
		&m.ID, &m.ConversationID, &m.Seq, &m.SenderID, &m.ClientTmpID,
		&m.Type, &m.Body, &m.ReplyToID, &m.CreatedAt, &m.EditedAt, &m.ForwardedFromMessageID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrMessageDeleted
	}
	return m, err
}

func (r *Repository) DeleteMessageForEveryone(ctx context.Context, messageID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE messages SET deleted_for_everyone_at = now(), body = NULL WHERE id = $1 AND deleted_for_everyone_at IS NULL`,
		messageID)
	return err
}

func (r *Repository) DeleteMessageForUser(ctx context.Context, messageID, userID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO message_deletions (message_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		messageID, userID)
	return err
}

// SetPinned / SetArchived toggle per-member list state.
func (r *Repository) SetPinned(ctx context.Context, conversationID, userID string, on bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE conversation_members
		SET pinned_at = CASE WHEN $3 THEN COALESCE(pinned_at, now()) ELSE NULL END
		WHERE conversation_id = $1 AND user_id = $2 AND left_at IS NULL`,
		conversationID, userID, on)
	return err
}

func (r *Repository) SetArchived(ctx context.Context, conversationID, userID string, on bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE conversation_members
		SET archived_at = CASE WHEN $3 THEN COALESCE(archived_at, now()) ELSE NULL END
		WHERE conversation_id = $1 AND user_id = $2 AND left_at IS NULL`,
		conversationID, userID, on)
	return err
}

// attachReactions fills Message.Reactions for a page of messages with one query.
func (r *Repository) attachReactions(ctx context.Context, msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]string, len(msgs))
	idx := make(map[string]int, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
		idx[m.ID] = i
	}
	rows, err := r.pool.Query(ctx,
		`SELECT message_id::text, emoji, user_id::text FROM message_reactions WHERE message_id::text = ANY($1) ORDER BY created_at ASC`,
		ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	type key struct{ msg, emoji string }
	pos := map[key]int{}
	for rows.Next() {
		var mid, emoji, uid string
		if err := rows.Scan(&mid, &emoji, &uid); err != nil {
			return err
		}
		i, ok := idx[mid]
		if !ok {
			continue
		}
		k := key{mid, emoji}
		j, seen := pos[k]
		if !seen {
			msgs[i].Reactions = append(msgs[i].Reactions, ReactionSummary{Emoji: emoji})
			j = len(msgs[i].Reactions) - 1
			pos[k] = j
		}
		msgs[i].Reactions[j].Users = append(msgs[i].Reactions[j].Users, uid)
	}
	return rows.Err()
}

// attachAttachments fills Message.Attachments/AttachmentIDs for a page of
// messages with one query. Deleted-for-everyone messages get none.
func (r *Repository) attachAttachments(ctx context.Context, msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(msgs))
	idx := make(map[string]int, len(msgs))
	for i, m := range msgs {
		if m.DeletedAt != nil {
			continue
		}
		ids = append(ids, m.ID)
		idx[m.ID] = i
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT a.message_id::text, f.id::text, f.kind, COALESCE(f.confirmed_mime_type, f.declared_mime_type),
		       COALESCE(f.size_bytes, 0), COALESCE(f.original_name, '')
		FROM message_attachments a
		JOIN media_files f ON f.id = a.media_id
		WHERE a.message_id::text = ANY($1) AND f.deleted_at IS NULL
		ORDER BY a.message_id, a.position ASC`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var mid string
		var ai AttachmentInfo
		if err := rows.Scan(&mid, &ai.ID, &ai.Kind, &ai.MimeType, &ai.SizeBytes, &ai.Name); err != nil {
			return err
		}
		i, ok := idx[mid]
		if !ok {
			continue
		}
		msgs[i].Attachments = append(msgs[i].Attachments, ai)
		msgs[i].AttachmentIDs = append(msgs[i].AttachmentIDs, ai.ID)
	}
	return rows.Err()
}

// attachStatus fills Message.Status for the requester's own, not-deleted
// messages with one query: "read" once every other member's last_read_seq has
// reached the message, "delivered" once any other member read it or has a
// delivery receipt, otherwise "sent".
func (r *Repository) attachStatus(ctx context.Context, msgs []Message, userID string) error {
	ids := make([]string, 0, len(msgs))
	idx := make(map[string]int, len(msgs))
	for i, m := range msgs {
		if m.SenderID == userID && m.DeletedAt == nil {
			ids = append(ids, m.ID)
			idx[m.ID] = i
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT m.id::text,
		       (SELECT COUNT(*) FROM conversation_members cm WHERE cm.conversation_id = m.conversation_id AND cm.left_at IS NULL AND cm.user_id <> m.sender_id),
		       (SELECT COUNT(*) FROM conversation_members cm WHERE cm.conversation_id = m.conversation_id AND cm.left_at IS NULL AND cm.user_id <> m.sender_id AND cm.last_read_seq >= m.seq),
		       (SELECT COUNT(*) FROM message_deliveries d WHERE d.message_id = m.id AND d.user_id <> m.sender_id)
		FROM messages m
		WHERE m.id::text = ANY($1)`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var others, readers, delivered int64
		if err := rows.Scan(&id, &others, &readers, &delivered); err != nil {
			return err
		}
		st := "sent"
		switch {
		case others > 0 && readers == others:
			st = "read"
		case readers > 0 || delivered > 0:
			st = "delivered"
		}
		msgs[idx[id]].Status = st
	}
	return rows.Err()
}
