package ai

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateConversation(ctx context.Context, userID, title, model string) (Conversation, error) {
	var c Conversation
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ai_conversations (user_id, title, model)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, title, model, created_at, updated_at
	`, userID, title, model).Scan(&c.ID, &c.UserID, &c.Title, &c.Model, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// GetOwned returns the conversation only if it belongs to userID — the
// object-level authorization check for every other method in this file
// that takes a conversationID.
func (r *Repository) GetOwned(ctx context.Context, userID, conversationID string) (Conversation, error) {
	var c Conversation
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, title, model, created_at, updated_at
		FROM ai_conversations
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, conversationID, userID).Scan(&c.ID, &c.UserID, &c.Title, &c.Model, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	return c, err
}

func (r *Repository) ListConversations(ctx context.Context, userID string) ([]Conversation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, title, model, created_at, updated_at
		FROM ai_conversations
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY updated_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.UserID, &c.Title, &c.Model, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateTitle(ctx context.Context, userID, conversationID, title string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE ai_conversations SET title = $1, updated_at = now()
		WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL
	`, title, conversationID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) SoftDelete(ctx context.Context, userID, conversationID string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE ai_conversations SET deleted_at = now()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, conversationID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) touchUpdatedAt(ctx context.Context, conversationID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE ai_conversations SET updated_at = now() WHERE id = $1`, conversationID)
	return err
}

// ListMessages returns the full turn history in chronological order — the
// context sent to Gemini on every new turn (see service.go buildHistory).
func (r *Repository) ListMessages(ctx context.Context, conversationID string) ([]Message, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, conversation_id, role, content, finish_reason, error, created_at
		FROM ai_messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC, id ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.FinishReason, &m.Error, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) AppendMessage(ctx context.Context, conversationID string, role Role, content string, finishReason, errText *string) (Message, error) {
	var m Message
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ai_messages (conversation_id, role, content, finish_reason, error)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, conversation_id, role, content, finish_reason, error, created_at
	`, conversationID, role, content, finishReason, errText).Scan(
		&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.FinishReason, &m.Error, &m.CreatedAt)
	if err != nil {
		return Message{}, err
	}
	_ = r.touchUpdatedAt(ctx, conversationID)
	return m, nil
}
