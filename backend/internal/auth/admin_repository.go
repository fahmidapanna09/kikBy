package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// This file is the small, deliberate surface auth exposes to the `admin`
// module (Phase 11) — see internal/admin/service.go's UserDirectory /
// UserModerator interfaces, which admin defines using only primitive
// types so admin can be satisfied by *Service structurally without admin
// importing this package, and without this package importing admin.
// Everything here is plain user-account data; it never touches message,
// conversation, or any other module's tables.

// GetUserIDByPhone is used only by the admin module's startup bootstrap
// (promoting a configured phone number to superadmin on first boot) — see
// cmd/api/main.go.
func (r *Repository) GetUserIDByPhone(ctx context.Context, phoneNumber string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE phone_number = $1 AND deleted_at IS NULL`, phoneNumber,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

func (r *Repository) GetByID(ctx context.Context, userID string) (User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`SELECT id, COALESCE(phone_number,''), COALESCE(country_code,''), username, status, created_at
		 FROM users WHERE id = $1 AND deleted_at IS NULL`, userID,
	).Scan(&u.ID, &u.PhoneNumber, &u.CountryCode, &u.Username, &u.Status, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// SearchUsersAdmin matches on an exact phone number or a case-insensitive
// username prefix — never a substring search over phone numbers, which
// would make it too easy to fish for registered numbers one digit at a
// time (the same "never expose phone numbers via search" concern from the
// contacts module's username search, Phase 5).
func (r *Repository) SearchUsersAdmin(ctx context.Context, query string, limit, offset int) ([]User, int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, COALESCE(phone_number,''), COALESCE(country_code,''), username, status, created_at, count(*) OVER() AS total
		FROM users
		WHERE deleted_at IS NULL AND (phone_number = $1 OR lower(username) LIKE lower($1) || '%')
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []User
	total := 0
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.PhoneNumber, &u.CountryCode, &u.Username, &u.Status, &u.CreatedAt, &total); err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// SetUserStatus updates users.status and, for a suspension, the
// suspended_* audit columns; unsuspending clears them back to NULL rather
// than keeping a stale reason/actor around from a past suspension.
func (r *Repository) SetUserStatus(ctx context.Context, userID, status string, reason, suspendedBy *string) error {
	var err error
	if status == "suspended" {
		_, err = r.pool.Exec(ctx, `
			UPDATE users SET status = $1, suspended_reason = $2, suspended_at = now(), suspended_by = $3, updated_at = now()
			WHERE id = $4 AND deleted_at IS NULL
		`, status, reason, suspendedBy, userID)
	} else {
		_, err = r.pool.Exec(ctx, `
			UPDATE users SET status = $1, suspended_reason = NULL, suspended_at = NULL, suspended_by = NULL, updated_at = now()
			WHERE id = $2 AND deleted_at IS NULL
		`, status, userID)
	}
	return err
}
