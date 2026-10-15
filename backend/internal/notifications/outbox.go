// Package notifications implements the push-notification outbox: chat (and
// later calls, groups, stories, etc.) enqueue a row instead of calling FCM
// directly, and a separate worker process drains it. This means a crashed
// or slow FCM call never blocks a message send, and a crashed worker never
// silently drops a notification — it's just still sitting in the table.
package notifications

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxItem struct {
	ID       string
	UserID   string
	Kind     string
	Payload  json.RawMessage
	Attempts int
}

type Outbox struct {
	pool *pgxpool.Pool
}

func NewOutbox(pool *pgxpool.Pool) *Outbox {
	return &Outbox{pool: pool}
}

func (o *Outbox) Enqueue(ctx context.Context, userID, kind string, payload json.RawMessage) error {
	_, err := o.pool.Exec(ctx,
		`INSERT INTO notification_outbox (user_id, kind, payload) VALUES ($1, $2, $3)`,
		userID, kind, payload,
	)
	return err
}

// ClaimBatch atomically claims up to `limit` pending rows for processing
// using SKIP LOCKED, so multiple worker replicas can run concurrently
// without double-sending the same notification.
func (o *Outbox) ClaimBatch(ctx context.Context, limit int) ([]OutboxItem, error) {
	rows, err := o.pool.Query(ctx, `
		UPDATE notification_outbox
		SET status = 'sending', attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM notification_outbox
			WHERE status = 'pending'
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, user_id, kind, payload, attempts
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OutboxItem
	for rows.Next() {
		var item OutboxItem
		if err := rows.Scan(&item.ID, &item.UserID, &item.Kind, &item.Payload, &item.Attempts); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (o *Outbox) MarkSent(ctx context.Context, id string) error {
	_, err := o.pool.Exec(ctx,
		`UPDATE notification_outbox SET status = 'sent', sent_at = $2 WHERE id = $1`,
		id, time.Now().UTC(),
	)
	return err
}

// MarkFailed returns it to 'pending' for retry unless attempts is already
// high, in which case it's parked as 'failed' so it stops being reclaimed
// forever (visible for manual/alerting follow-up instead).
func (o *Outbox) MarkFailed(ctx context.Context, id string, attempts int) error {
	status := "pending"
	if attempts >= 5 {
		status = "failed"
	}
	_, err := o.pool.Exec(ctx, `UPDATE notification_outbox SET status = $2 WHERE id = $1`, id, status)
	return err
}
