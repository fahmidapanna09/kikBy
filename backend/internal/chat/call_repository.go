package chat

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const callCols = `id, conversation_id, initiator_id, kind, status, started_at, answered_at, ended_at, COALESCE(end_reason, '')`

func scanCall(row pgx.Row) (Call, error) {
	var c Call
	err := row.Scan(&c.ID, &c.ConversationID, &c.InitiatorID, &c.Kind, &c.Status, &c.StartedAt, &c.AnsweredAt, &c.EndedAt, &c.EndReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return Call{}, ErrCallNotFound
	}
	return c, err
}

// ActiveMemberIDs lists everybody currently in the conversation (including the caller).
func (r *Repository) ActiveMemberIDs(ctx context.Context, conversationID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id FROM conversation_members WHERE conversation_id = $1 AND left_at IS NULL`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ExpireStaleCalls closes calls that can never finish on their own: a ringing call nobody answered for over a minute (missed),
// a live call with nobody in it (ended), or anything live for over 12 hours.
func (r *Repository) ExpireStaleCalls(ctx context.Context, conversationID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE calls c SET
		  status = CASE WHEN c.status = 'ringing' THEN 'missed' ELSE 'ended' END,
		  ended_at = now(),
		  end_reason = 'stale'
		WHERE c.conversation_id = $1 AND c.status IN ('ringing', 'active')
		  AND (
		    c.started_at < now() - interval '12 hours'
		    OR NOT EXISTS (SELECT 1 FROM call_participants p WHERE p.call_id = c.id AND p.left_at IS NULL)
		    OR (c.status = 'ringing' AND c.started_at < now() - interval '60 seconds')
		  )`, conversationID)
	return err
}

func (r *Repository) LiveCall(ctx context.Context, conversationID string) (Call, bool, error) {
	c, err := scanCall(r.pool.QueryRow(ctx, `SELECT `+callCols+` FROM calls WHERE conversation_id = $1 AND status IN ('ringing', 'active')`, conversationID))
	if errors.Is(err, ErrCallNotFound) {
		return Call{}, false, nil
	}
	return c, err == nil, err
}

func (r *Repository) CreateCall(ctx context.Context, conversationID, initiatorID, kind string) (Call, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Call{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	c, err := scanCall(tx.QueryRow(ctx,
		`INSERT INTO calls (conversation_id, initiator_id, kind) VALUES ($1, $2, $3) RETURNING `+callCols,
		conversationID, initiatorID, kind))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Call{}, ErrCallLive
		}
		return Call{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO call_participants (call_id, user_id) VALUES ($1, $2)`, c.ID, initiatorID); err != nil {
		return Call{}, err
	}
	return c, tx.Commit(ctx)
}

func (r *Repository) GetCall(ctx context.Context, callID string) (Call, error) {
	return scanCall(r.pool.QueryRow(ctx, `SELECT `+callCols+` FROM calls WHERE id = $1`, callID))
}

// JoinCall adds (or re-adds) a participant under a row lock on the call, enforcing the size cap, and flips ringing -> active
// when someone other than the initiator joins.
func (r *Repository) JoinCall(ctx context.Context, callID, userID string, max int) (Call, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Call{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	c, err := scanCall(tx.QueryRow(ctx, `SELECT `+callCols+` FROM calls WHERE id = $1 FOR UPDATE`, callID))
	if err != nil {
		return Call{}, err
	}
	if !c.Live() {
		return Call{}, ErrCallEnded
	}
	var others int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM call_participants WHERE call_id = $1 AND left_at IS NULL AND user_id <> $2`, callID, userID).Scan(&others); err != nil {
		return Call{}, err
	}
	if others+1 > max {
		return Call{}, ErrCallFull
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO call_participants (call_id, user_id) VALUES ($1, $2)
		ON CONFLICT (call_id, user_id) DO UPDATE SET left_at = NULL, joined_at = now()
		WHERE call_participants.left_at IS NOT NULL`, callID, userID); err != nil {
		return Call{}, err
	}
	if c.Status == CallRinging && userID != c.InitiatorID {
		now := time.Now().UTC()
		if _, err := tx.Exec(ctx, `UPDATE calls SET status = 'active', answered_at = $2 WHERE id = $1`, callID, now); err != nil {
			return Call{}, err
		}
		c.Status, c.AnsweredAt = CallActive, &now
	}
	return c, tx.Commit(ctx)
}

func (r *Repository) LeaveCall(ctx context.Context, callID, userID string) (int, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM calls WHERE id = $1 FOR UPDATE`, callID); err != nil {
		return 0, false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE call_participants SET left_at = now() WHERE call_id = $1 AND user_id = $2 AND left_at IS NULL`, callID, userID)
	if err != nil {
		return 0, false, err
	}
	var remaining int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM call_participants WHERE call_id = $1 AND left_at IS NULL`, callID).Scan(&remaining); err != nil {
		return 0, false, err
	}
	return remaining, tag.RowsAffected() > 0, tx.Commit(ctx)
}

func (r *Repository) EndCall(ctx context.Context, callID, status, reason, requireStatus string) (Call, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Call{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	c, err := scanCall(tx.QueryRow(ctx, `SELECT `+callCols+` FROM calls WHERE id = $1 FOR UPDATE`, callID))
	if err != nil {
		return Call{}, false, err
	}
	if !c.Live() || (requireStatus != "" && c.Status != requireStatus) {
		return c, false, nil
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE calls SET status = $2, end_reason = $3, ended_at = $4 WHERE id = $1`, callID, status, reason, now); err != nil {
		return Call{}, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE call_participants SET left_at = $2 WHERE call_id = $1 AND left_at IS NULL`, callID, now); err != nil {
		return Call{}, false, err
	}
	c.Status, c.EndReason, c.EndedAt = status, reason, &now
	return c, true, tx.Commit(ctx)
}

func (r *Repository) JoinedUsers(ctx context.Context, callID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id FROM call_participants WHERE call_id = $1 AND left_at IS NULL ORDER BY joined_at`, callID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repository) CallsFor(ctx context.Context, conversationID string, limit int) ([]Call, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+callCols+` FROM calls WHERE conversation_id = $1 ORDER BY started_at DESC LIMIT $2`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Call{}
	for rows.Next() {
		c, err := scanCall(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) LiveCallIDsOfUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.call_id FROM call_participants p JOIN calls c ON c.id = p.call_id
		WHERE p.user_id = $1 AND p.left_at IS NULL AND c.status IN ('ringing', 'active')`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
