// Package inbox is the in-app notification list ("bell" screen): a durable per-user feed that is
// separate from push (internal/notifications, FCM). Rows are written by other modules through
// Service.Notify; clients read them over REST and receive "notification.new" live over the WebSocket hub.
package inbox

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("inbox: notification not found")

const EventNotificationNew = "notification.new"

type Notification struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"` // message | friend_request | friend_accept | mention | system
	Title          string    `json:"title"`
	Body           string    `json:"body"`
	ActorID        *string   `json:"actor_id,omitempty"`
	ConversationID *string   `json:"conversation_id,omitempty"`
	Read           bool      `json:"read"`
	CreatedAt      time.Time `json:"created_at"`
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const cols = `id, kind, title, body, actor_id, conversation_id, read_at IS NOT NULL, created_at`

func scan(row pgx.Row) (Notification, error) {
	var n Notification
	err := row.Scan(&n.ID, &n.Kind, &n.Title, &n.Body, &n.ActorID, &n.ConversationID, &n.Read, &n.CreatedAt)
	return n, err
}

func (r *Repository) Create(ctx context.Context, userID, kind, title, body string, actorID, conversationID *string) (Notification, error) {
	return scan(r.pool.QueryRow(ctx, `
		INSERT INTO app_notifications (user_id, kind, title, body, actor_id, conversation_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+cols, userID, kind, title, body, actorID, conversationID))
}

func (r *Repository) List(ctx context.Context, userID string, limit int) ([]Notification, int, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+cols+` FROM app_notifications WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		n, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var unread int
	err = r.pool.QueryRow(ctx, `SELECT count(*) FROM app_notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&unread)
	return out, unread, err
}

// MarkRead only touches the caller's own rows, so a guessed id of someone else's notification is a plain 404.
func (r *Repository) MarkRead(ctx context.Context, userID, id string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE app_notifications SET read_at = COALESCE(read_at, now()) WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) MarkAllRead(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE app_notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, userID)
	return err
}

// Publisher is satisfied by *chat.Hub (PublishEvent) without importing the chat package.
type Publisher interface {
	PublishEvent(ctx context.Context, userID, eventType string, payload any) error
}

type Service struct {
	repo *Repository
	pub  Publisher // may be nil: rows are still stored, just not pushed live
}

func NewService(repo *Repository, pub Publisher) *Service { return &Service{repo: repo, pub: pub} }

// Notify stores the notification and, best-effort, pushes it to the user's open sockets. A failed live push never
// fails the caller: the row is already durable and shows up on the next list.
func (s *Service) Notify(ctx context.Context, userID, kind, title, body string, actorID, conversationID *string) error {
	n, err := s.repo.Create(ctx, userID, kind, title, body, actorID, conversationID)
	if err != nil {
		return err
	}
	if s.pub != nil {
		_ = s.pub.PublishEvent(ctx, userID, EventNotificationNew, map[string]any{"notification": n})
	}
	return nil
}

func (s *Service) List(ctx context.Context, userID string) ([]Notification, int, error) {
	return s.repo.List(ctx, userID, 100)
}
func (s *Service) MarkRead(ctx context.Context, userID, id string) error {
	return s.repo.MarkRead(ctx, userID, id)
}
func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	return s.repo.MarkAllRead(ctx, userID)
}
