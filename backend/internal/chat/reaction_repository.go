package chat

import "context"

// SetReaction upserts a user's reaction on a message — one reaction slot
// per (message, user); setting a new emoji replaces whatever was there.
func (r *Repository) SetReaction(ctx context.Context, messageID, userID, emoji string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO message_reactions (message_id, user_id, emoji) VALUES ($1, $2, $3)
		ON CONFLICT (message_id, user_id) DO UPDATE SET emoji = EXCLUDED.emoji, created_at = now()
	`, messageID, userID, emoji)
	return err
}

func (r *Repository) RemoveReaction(ctx context.Context, messageID, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM message_reactions WHERE message_id = $1 AND user_id = $2`, messageID, userID)
	return err
}

// ListReactions groups reactions by emoji (what the UI wants: "👍 3, ❤️ 1"),
// each with the list of user ids who used it.
func (r *Repository) ListReactions(ctx context.Context, messageID string) ([]ReactionSummary, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT emoji, user_id FROM message_reactions WHERE message_id = $1 ORDER BY created_at ASC`,
		messageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	order := []string{}
	byEmoji := map[string][]string{}
	for rows.Next() {
		var emoji, userID string
		if err := rows.Scan(&emoji, &userID); err != nil {
			return nil, err
		}
		if _, seen := byEmoji[emoji]; !seen {
			order = append(order, emoji)
		}
		byEmoji[emoji] = append(byEmoji[emoji], userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]ReactionSummary, 0, len(order))
	for _, emoji := range order {
		out = append(out, ReactionSummary{Emoji: emoji, Users: byEmoji[emoji]})
	}
	return out, nil
}

// ConversationIDForMessage is needed by the reaction handler to fan out the
// reaction event and to check the reactor is actually a member — a
// reaction's own row doesn't carry conversation_id, only its message does.
func (r *Repository) ConversationIDForMessage(ctx context.Context, messageID string) (string, error) {
	var conversationID string
	err := r.pool.QueryRow(ctx, `SELECT conversation_id FROM messages WHERE id = $1`, messageID).Scan(&conversationID)
	return conversationID, err
}
