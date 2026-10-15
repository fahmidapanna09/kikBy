package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrEmailTaken    = errors.New("auth: email already registered")
	ErrUsernameTaken = errors.New("auth: username taken")
)

// CreateUserWithEmail inserts users + user_profiles atomically. Unique
// violations are mapped to ErrEmailTaken / ErrUsernameTaken by constraint
// (index) name, so the caller can report them on the right form field.
func (r *Repository) CreateUserWithEmail(ctx context.Context, username, displayName, email, passwordHash, contactPhone string) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)

	var u User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash, contact_phone)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		RETURNING id, username, COALESCE(email, ''), status, created_at
	`, username, email, passwordHash, contactPhone).Scan(&u.ID, &u.Username, &u.Email, &u.Status, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "uq_users_email":
				return User{}, ErrEmailTaken
			case "uq_users_username":
				return User{}, ErrUsernameTaken
			}
		}
		return User{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_profiles (user_id, display_name) VALUES ($1, $2)`, u.ID, displayName); err != nil {
		return User{}, err
	}
	return u, tx.Commit(ctx)
}

// GetCredentialsByIdentifier looks a user up by lower-cased e-mail or username.
// passwordHash is "" for legacy phone/OTP-only accounts (they cannot use password login).
func (r *Repository) GetCredentialsByIdentifier(ctx context.Context, identifier string) (u User, passwordHash string, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT id, COALESCE(phone_number,''), COALESCE(country_code,''), username, COALESCE(email,''), status, created_at, COALESCE(password_hash,''), email_verified_at IS NOT NULL
		FROM users
		WHERE deleted_at IS NULL AND (email = $1 OR lower(username) = $1)
		LIMIT 1
	`, identifier).Scan(&u.ID, &u.PhoneNumber, &u.CountryCode, &u.Username, &u.Email, &u.Status, &u.CreatedAt, &passwordHash, &u.EmailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	return u, passwordHash, err
}

func (r *Repository) GetUserIDByEmail(ctx context.Context, email string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1 AND deleted_at IS NULL AND status = 'active'`, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// CreateResetToken invalidates any still-open tokens for the user and stores
// the hash of a new one, so only the most recent e-mailed link works.
func (r *Repository) CreateResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE password_reset_tokens SET used_at = now() WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO password_reset_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`, userID, tokenHash, expiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ConsumeResetToken atomically marks a valid, unused, unexpired token as used
// and returns its user. Two concurrent requests with the same token cannot
// both succeed (the UPDATE ... WHERE used_at IS NULL is the arbiter).
func (r *Repository) ConsumeResetToken(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := r.pool.QueryRow(ctx, `
		UPDATE password_reset_tokens SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id
	`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

// SetPasswordAndRevokeSessions changes the hash and logs the account out
// everywhere in one transaction (a reset usually means "I may be compromised").
// Following the e-mailed reset link also proves control of the address, so it counts as verification.
func (r *Repository) SetPasswordAndRevokeSessions(ctx context.Context, userID, passwordHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE users SET password_hash = $2, email_verified_at = COALESCE(email_verified_at, now()) WHERE id = $1`, userID, passwordHash); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE user_sessions SET revoked_at = now(), revoked_reason = 'password_reset' WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
