package users

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("users: not found")
	ErrUsernameTaken = errors.New("users: username already taken")
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetMe(ctx context.Context, userID string) (MeResponse, error) {
	var m MeResponse
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, COALESCE(u.phone_number,''), COALESCE(u.email,''), u.email_verified_at IS NOT NULL, COALESCE(u.pending_email,''), COALESCE(u.contact_phone,''), u.username, p.display_name, p.about, p.avatar_media_id, p.last_seen_visibility
		FROM users u JOIN user_profiles p ON p.user_id = u.id
		WHERE u.id = $1 AND u.deleted_at IS NULL
	`, userID).Scan(&m.UserID, &m.PhoneNumber, &m.Email, &m.EmailVerified, &m.PendingEmail, &m.ContactPhone, &m.Username, &m.DisplayName, &m.About, &m.AvatarMediaID, &m.LastSeenVisibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return MeResponse{}, ErrNotFound
	}
	return m, err
}

func (r *Repository) GetPublicProfile(ctx context.Context, userID string) (PublicProfile, error) {
	var p PublicProfile
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, u.username, prof.display_name, prof.about, prof.avatar_media_id
		FROM users u JOIN user_profiles prof ON prof.user_id = u.id
		WHERE u.id = $1 AND u.deleted_at IS NULL
	`, userID).Scan(&p.UserID, &p.Username, &p.DisplayName, &p.About, &p.AvatarMediaID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicProfile{}, ErrNotFound
	}
	return p, err
}

// GetPublicProfileByPhone does an EXACT match only (never a prefix/LIKE
// search) — that's what keeps phone-number lookup from being usable to
// enumerate the user base; combined with the caller's rate limiting (see
// service.go), guessing numbers one at a time is impractical.
func (r *Repository) GetPublicProfileByPhone(ctx context.Context, phoneNumber string) (PublicProfile, error) {
	var p PublicProfile
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, u.username, prof.display_name, prof.about, prof.avatar_media_id
		FROM users u JOIN user_profiles prof ON prof.user_id = u.id
		WHERE u.phone_number = $1 AND u.deleted_at IS NULL
	`, phoneNumber).Scan(&p.UserID, &p.Username, &p.DisplayName, &p.About, &p.AvatarMediaID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicProfile{}, ErrNotFound
	}
	return p, err
}

// SearchByUsername does a prefix match — unlike phone search, usernames are
// meant to be discoverable, so a broader search is the intended product
// behavior (same as Telegram/Instagram-style username search), not a
// privacy leak the way phone-prefix search would be.
func (r *Repository) SearchByUsername(ctx context.Context, prefix string, limit int) ([]PublicProfile, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.username, prof.display_name, prof.about, prof.avatar_media_id
		FROM users u JOIN user_profiles prof ON prof.user_id = u.id
		WHERE u.username ILIKE $1 || '%' AND u.deleted_at IS NULL
		ORDER BY u.username ASC
		LIMIT $2
	`, prefix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PublicProfile
	for rows.Next() {
		var p PublicProfile
		if err := rows.Scan(&p.UserID, &p.Username, &p.DisplayName, &p.About, &p.AvatarMediaID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) error {
	if req.ContactPhone != nil {
		if _, err := r.pool.Exec(ctx, `UPDATE users SET contact_phone = NULLIF($2, '') WHERE id = $1`, userID, *req.ContactPhone); err != nil {
			return err
		}
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE user_profiles SET
			display_name = COALESCE($2, display_name),
			about = COALESCE($3, about),
			avatar_media_id = COALESCE($4, avatar_media_id),
			last_seen_visibility = COALESCE($5, last_seen_visibility)
		WHERE user_id = $1
	`, userID, req.DisplayName, req.About, req.AvatarMediaID, req.LastSeenVisibility)
	return err
}

func (r *Repository) UpdateUsername(ctx context.Context, userID, newUsername string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET username = $2 WHERE id = $1`, userID, newUsername)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrUsernameTaken
	}
	return err
}

func (r *Repository) ReportUser(ctx context.Context, reporterID, reportedID, reason string, contextMessageID *string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO reported_users (reporter_id, reported_id, reason, context_message_id) VALUES ($1, $2, $3, $4)`,
		reporterID, reportedID, reason, contextMessageID,
	)
	return err
}
