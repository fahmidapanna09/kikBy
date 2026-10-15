package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("auth: not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// FindOrCreateUserByPhone is called on successful OTP verification. It's
// transactional so a concurrent duplicate registration for the same number
// can't race past the unique index.
func (r *Repository) FindOrCreateUserByPhone(ctx context.Context, phoneNumber, countryCode string) (User, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, false, err
	}
	defer tx.Rollback(ctx)

	var u User
	err = tx.QueryRow(ctx,
		`SELECT id, phone_number, country_code, username, status, created_at
		 FROM users WHERE phone_number = $1 AND deleted_at IS NULL`,
		phoneNumber,
	).Scan(&u.ID, &u.PhoneNumber, &u.CountryCode, &u.Username, &u.Status, &u.CreatedAt)

	if err == nil {
		return u, false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, err
	}

	phoneHash := hashPhoneNumber(phoneNumber)

	// New account: generate a username and insert, retrying with a fresh
	// random suffix if it happens to collide (the "user_" + 10-hex-char
	// space is large enough that this should almost never fire more than
	// once). Each attempt runs inside its own SAVEPOINT (pgx's Tx.Begin,
	// called on an existing Tx, creates one) — a plain retry in the same
	// transaction wouldn't work, since Postgres aborts the whole
	// transaction on the first error and refuses further commands until
	// it's rolled back; the savepoint scopes that abort to just the one
	// failed attempt.
	const maxUsernameAttempts = 5
	for attempt := 0; attempt < maxUsernameAttempts; attempt++ {
		username, genErr := generateUsername()
		if genErr != nil {
			return User{}, false, genErr
		}

		savepoint, spErr := tx.Begin(ctx)
		if spErr != nil {
			return User{}, false, spErr
		}

		err = savepoint.QueryRow(ctx,
			`INSERT INTO users (phone_number, country_code, phone_number_hash, username)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id, phone_number, country_code, username, status, created_at`,
			phoneNumber, countryCode, phoneHash, username,
		).Scan(&u.ID, &u.PhoneNumber, &u.CountryCode, &u.Username, &u.Status, &u.CreatedAt)

		if err == nil {
			if commitErr := savepoint.Commit(ctx); commitErr != nil {
				return User{}, false, commitErr
			}
			break
		}

		_ = savepoint.Rollback(ctx) // rolls back to the savepoint; the outer tx is still usable
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_users_username" {
			continue // username collision — try another random one
		}
		return User{}, false, err // any other error (including a phone-number race) is not retryable here
	}
	if err != nil {
		return User{}, false, err // exhausted retries — vanishingly unlikely, but don't loop forever
	}

	if _, err = tx.Exec(ctx, `INSERT INTO user_profiles (user_id) VALUES ($1)`, u.ID); err != nil {
		return User{}, false, err
	}

	if err = tx.Commit(ctx); err != nil {
		return User{}, false, err
	}
	return u, true, nil
}

// UpsertDevice registers or refreshes a device for a user. device_id is the
// client-generated stable identifier (not the DB row ID).
func (r *Repository) UpsertDevice(ctx context.Context, userID, deviceID, platform, pushToken, appVersion string) (Device, error) {
	var d Device
	err := r.pool.QueryRow(ctx, `
		INSERT INTO user_devices (user_id, device_id, platform, push_token, app_version, last_active_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), now())
		ON CONFLICT (user_id, device_id) DO UPDATE
		  SET platform = EXCLUDED.platform,
		      push_token = COALESCE(EXCLUDED.push_token, user_devices.push_token),
		      app_version = COALESCE(EXCLUDED.app_version, user_devices.app_version),
		      last_active_at = now(),
		      revoked_at = NULL
		RETURNING id, user_id, device_id, platform, push_token, app_version
	`, userID, deviceID, platform, pushToken, appVersion).Scan(
		&d.ID, &d.UserID, &d.DeviceID, &d.Platform, &d.PushToken, &d.AppVersion,
	)
	return d, err
}

// CreateSession stores a hashed refresh token tied to a device.
func (r *Repository) CreateSession(ctx context.Context, userID, deviceRowID, refreshTokenHash, ip, userAgent string, expiresAt time.Time, parentSessionID *string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO user_sessions (user_id, device_id, refresh_token_hash, parent_session_id, ip_address, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, NULLIF($5,'')::inet, $6, $7)
		RETURNING id
	`, userID, deviceRowID, refreshTokenHash, parentSessionID, ip, userAgent, expiresAt).Scan(&id)
	return id, err
}

type sessionRow struct {
	ID               string
	UserID           string
	DeviceRowID      string
	RefreshTokenHash string
	ExpiresAt        time.Time
	RevokedAt        *time.Time
}

func (r *Repository) GetSessionByRefreshHash(ctx context.Context, hash string) (sessionRow, error) {
	var s sessionRow
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, device_id, refresh_token_hash, expires_at, revoked_at
		FROM user_sessions WHERE refresh_token_hash = $1
	`, hash).Scan(&s.ID, &s.UserID, &s.DeviceRowID, &s.RefreshTokenHash, &s.ExpiresAt, &s.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return sessionRow{}, ErrNotFound
	}
	return s, err
}

func (r *Repository) RevokeSession(ctx context.Context, sessionID, reason string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE user_sessions SET revoked_at = now(), revoked_reason = $2 WHERE id = $1 AND revoked_at IS NULL`,
		sessionID, reason,
	)
	return err
}

// RevokeSessionForUser is object-level-authorization-safe: it only revokes
// the session if it actually belongs to userID, so an authenticated user
// cannot revoke another user's session by guessing/enumerating a session ID.
// Returns ErrNotFound if no matching, still-active session exists.
func (r *Repository) RevokeSessionForUser(ctx context.Context, sessionID, userID, reason string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE user_sessions SET revoked_at = now(), revoked_reason = $3
		 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		sessionID, userID, reason,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeAllSessionsForUser is used on reuse-detection (refresh token replay)
// as a defensive measure: if a rotated-out token is reused, the entire
// session chain for that device family is torn down.
func (r *Repository) RevokeAllSessionsForUser(ctx context.Context, userID, reason string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE user_sessions SET revoked_at = now(), revoked_reason = $2 WHERE user_id = $1 AND revoked_at IS NULL`,
		userID, reason,
	)
	return err
}

func (r *Repository) ListActiveSessions(ctx context.Context, userID string) ([]SessionInfo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, d.platform, COALESCE(host(s.ip_address), ''), COALESCE(s.user_agent, ''), s.issued_at
		FROM user_sessions s
		JOIN user_devices d ON d.id = s.device_id
		WHERE s.user_id = $1 AND s.revoked_at IS NULL AND s.expires_at > now()
		ORDER BY s.issued_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionInfo
	for rows.Next() {
		var si SessionInfo
		if err := rows.Scan(&si.ID, &si.Platform, &si.IPAddress, &si.UserAgent, &si.IssuedAt); err != nil {
			return nil, err
		}
		out = append(out, si)
	}
	return out, rows.Err()
}
