package notifications

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TokenRepository struct {
	pool *pgxpool.Pool
}

func NewTokenRepository(pool *pgxpool.Pool) *TokenRepository {
	return &TokenRepository{pool: pool}
}

// RegisterToken upserts by device_id: a device's FCM token can rotate
// (Firebase does this periodically) without creating duplicate rows.
func (t *TokenRepository) RegisterToken(ctx context.Context, userID, deviceRowID, fcmToken string) error {
	_, err := t.pool.Exec(ctx, `
		INSERT INTO push_tokens (user_id, device_id, fcm_token)
		VALUES ($1, $2, $3)
		ON CONFLICT (device_id) DO UPDATE SET fcm_token = EXCLUDED.fcm_token, updated_at = now()
	`, userID, deviceRowID, fcmToken)
	return err
}

func (t *TokenRepository) TokensForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := t.pool.Query(ctx, `SELECT fcm_token FROM push_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var tok string
		if err := rows.Scan(&tok); err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
	}
	return tokens, rows.Err()
}

func (t *TokenRepository) DeleteToken(ctx context.Context, fcmToken string) error {
	_, err := t.pool.Exec(ctx, `DELETE FROM push_tokens WHERE fcm_token = $1`, fcmToken)
	return err
}
