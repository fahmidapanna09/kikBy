package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("contacts: not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// AddContact upserts by (owner_id, phone_number) — re-adding the same
// number just updates the nickname rather than erroring, which matches how
// a phone's contact-sync naturally re-runs on every app open.
func (r *Repository) AddContact(ctx context.Context, ownerID, phoneNumber, nickname, addedVia string) (Contact, error) {
	var c Contact
	var nick *string
	if nickname != "" {
		nick = &nickname
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO contacts (owner_id, contact_user_id, phone_number, nickname, added_via)
		VALUES ($1, (SELECT id FROM users WHERE phone_number = $2 AND deleted_at IS NULL), $2, $3, $4)
		ON CONFLICT (owner_id, phone_number) DO UPDATE SET
			nickname = COALESCE(EXCLUDED.nickname, contacts.nickname),
			contact_user_id = (SELECT id FROM users WHERE phone_number = $2 AND deleted_at IS NULL)
		RETURNING id, owner_id, contact_user_id, phone_number, nickname, added_via, created_at
	`, ownerID, phoneNumber, nick, addedVia).Scan(
		&c.ID, &c.OwnerID, &c.ContactUserID, &c.PhoneNumber, &c.Nickname, &c.AddedVia, &c.CreatedAt,
	)
	return c, err
}

func (r *Repository) ListContacts(ctx context.Context, ownerID string) ([]ContactResponse, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.contact_user_id, c.phone_number, c.nickname,
		       u.username, p.display_name
		FROM contacts c
		LEFT JOIN users u ON u.id = c.contact_user_id AND u.deleted_at IS NULL
		LEFT JOIN user_profiles p ON p.user_id = c.contact_user_id
		WHERE c.owner_id = $1
		ORDER BY COALESCE(c.nickname, c.phone_number) ASC
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContactResponse
	for rows.Next() {
		var cr ContactResponse
		if err := rows.Scan(&cr.ID, &cr.ContactUserID, &cr.PhoneNumber, &cr.Nickname, &cr.Username, &cr.DisplayName); err != nil {
			return nil, err
		}
		cr.IsRegistered = cr.ContactUserID != nil
		out = append(out, cr)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateNickname(ctx context.Context, ownerID, contactID, nickname string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE contacts SET nickname = $3 WHERE id = $1 AND owner_id = $2`,
		contactID, ownerID, nickname,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteContact(ctx context.Context, ownerID, contactID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM contacts WHERE id = $1 AND owner_id = $2`, contactID, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MatchPhoneHashes is the core of privacy-preserving contact sync: it takes
// hashes the client computed locally and returns only the ones that match a
// registered account, plus that account's public info — the server never
// learns which of the requester's OTHER (non-matching) contacts exist at
// all, registered or not.
func (r *Repository) MatchPhoneHashes(ctx context.Context, phoneHashes []string) ([]SyncMatch, error) {
	if len(phoneHashes) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT u.phone_number_hash, u.id, u.username, p.display_name
		FROM users u
		JOIN user_profiles p ON p.user_id = u.id
		WHERE u.phone_number_hash = ANY($1) AND u.deleted_at IS NULL
	`, phoneHashes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SyncMatch
	for rows.Next() {
		var m SyncMatch
		if err := rows.Scan(&m.PhoneHash, &m.UserID, &m.Username, &m.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// --- Blocking ---

func (r *Repository) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO blocked_users (blocker_id, blocked_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		blockerID, blockedID,
	)
	return err
}

func (r *Repository) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM blocked_users WHERE blocker_id = $1 AND blocked_id = $2`, blockerID, blockedID)
	return err
}

func (r *Repository) IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM blocked_users WHERE blocker_id = $1 AND blocked_id = $2)`,
		blockerID, blockedID,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) ListBlocked(ctx context.Context, blockerID string) ([]BlockedUserResponse, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT b.blocked_id, u.username, p.display_name, b.created_at
		FROM blocked_users b
		JOIN users u ON u.id = b.blocked_id
		JOIN user_profiles p ON p.user_id = u.id
		WHERE b.blocker_id = $1
		ORDER BY b.created_at DESC
	`, blockerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BlockedUserResponse
	for rows.Next() {
		var b BlockedUserResponse
		if err := rows.Scan(&b.UserID, &b.Username, &b.DisplayName, &b.BlockedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
