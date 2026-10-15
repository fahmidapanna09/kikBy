package admin

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

func (r *Repository) GetRole(ctx context.Context, userID string) (Role, error) {
	var role string
	err := r.pool.QueryRow(ctx, `SELECT role FROM admin_roles WHERE user_id = $1`, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return Role(role), nil
}

// SetRole upserts — granting a role to someone who already has one simply
// changes it (e.g. promoting a moderator to admin), it doesn't require a
// separate revoke-then-grant.
func (r *Repository) SetRole(ctx context.Context, userID string, role Role, grantedBy string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO admin_roles (user_id, role, granted_by, granted_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (user_id) DO UPDATE SET role = $2, granted_by = $3, granted_at = now()
	`, userID, string(role), grantedBy)
	return err
}

func (r *Repository) RemoveRole(ctx context.Context, userID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM admin_roles WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ListAdmins(ctx context.Context) ([]AdminRoleInfo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT user_id, role, granted_by, granted_at FROM admin_roles ORDER BY granted_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AdminRoleInfo
	for rows.Next() {
		var info AdminRoleInfo
		var role string
		if err := rows.Scan(&info.UserID, &role, &info.GrantedBy, &info.GrantedAt); err != nil {
			return nil, err
		}
		info.Role = Role(role)
		out = append(out, info)
	}
	return out, rows.Err()
}

// CountAdmins is used only to stop the last superadmin from being removed
// via RemoveRole, so the system can never end up with zero superadmins
// and no way to grant any more roles. See Service.RevokeRole.
func (r *Repository) CountByRole(ctx context.Context, role Role) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM admin_roles WHERE role = $1`, string(role)).Scan(&n)
	return n, err
}
