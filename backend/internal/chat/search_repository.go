package chat

import "context"

// SearchMessages full-text-searches message bodies using Postgres's
// plainto_tsquery (handles multi-word queries as an AND of terms, safe
// against tsquery syntax injection since plainto_tsquery treats the input
// as plain text, not a query expression). Always scoped to conversations
// the requesting user is currently a member of — never searches messages
// from conversations they've left or were never part of. Optionally
// narrowed to one conversation.
func (r *Repository) SearchMessages(ctx context.Context, userID string, query string, conversationID *string, limit int) ([]Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.conversation_id, m.seq, m.sender_id, m.client_tmp_id, m.type, m.body, m.reply_to_id, m.created_at, m.edited_at, m.forwarded_from_message_id
		FROM messages m
		JOIN conversation_members cm ON cm.conversation_id = m.conversation_id AND cm.user_id = $1 AND cm.left_at IS NULL
		WHERE m.search_vector @@ plainto_tsquery('english', $2)
		  AND ($3::uuid IS NULL OR m.conversation_id = $3)
		  AND NOT EXISTS (SELECT 1 FROM message_deletions d WHERE d.message_id = m.id AND d.user_id = $1)
		ORDER BY m.created_at DESC
		LIMIT $4
	`, userID, query, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Seq, &m.SenderID, &m.ClientTmpID, &m.Type, &m.Body, &m.ReplyToID, &m.CreatedAt, &m.EditedAt, &m.ForwardedFromMessageID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
