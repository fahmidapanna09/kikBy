// Package friends implements follow / follow-back on top of the friendships table:
//
//	pending  (requester → addressee)  = requester follows addressee; addressee sees "follows you" and can follow back
//	accepted                          = both follow each other = friends (chat, presence, calls)
//
// Unfollowing a friend keeps the other person's follow alive (the row flips to pending in their direction).
// Blocking is stored in blocked_users (shared with contacts/stories) so every module sees one source of truth.
package friends

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("friends: not found")
	ErrBlocked   = errors.New("friends: you have blocked this user")
	ErrSelf      = errors.New("friends: cannot befriend yourself")
	ErrInvalidID = errors.New("friends: invalid user id")
)

const (
	StatusFriends  = "friends"
	StatusIncoming = "incoming"
	StatusOutgoing = "outgoing"
	StatusBlocked  = "blocked"
)

type UserLite struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	About       string `json:"about,omitempty"`
}

type Relation struct {
	User   UserLite   `json:"user"`
	Status string     `json:"status"`
	Since  *time.Time `json:"since,omitempty"`
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// relationsSQL lists every relationship of $1 (optionally narrowed to the other user $2). The "other" side
// is chosen with a CASE, and blocks I made are appended as status 'blocked'.
const relationsSQL = `
SELECT u.id::text, u.username, COALESCE(p.display_name, u.username), COALESCE(p.about, ''),
       CASE WHEN f.status = 'accepted' THEN 'friends' WHEN f.requester_id = $1::uuid THEN 'outgoing' ELSE 'incoming' END,
       f.responded_at
FROM friendships f
JOIN users u ON u.id = CASE WHEN f.requester_id = $1::uuid THEN f.addressee_id ELSE f.requester_id END AND u.deleted_at IS NULL
LEFT JOIN user_profiles p ON p.user_id = u.id
WHERE (f.requester_id = $1::uuid OR f.addressee_id = $1::uuid) AND ($2::uuid IS NULL OR u.id = $2::uuid)
UNION ALL
SELECT u.id::text, u.username, COALESCE(p.display_name, u.username), COALESCE(p.about, ''), 'blocked', b.created_at
FROM blocked_users b
JOIN users u ON u.id = b.blocked_id AND u.deleted_at IS NULL
LEFT JOIN user_profiles p ON p.user_id = u.id
WHERE b.blocker_id = $1::uuid AND ($2::uuid IS NULL OR u.id = $2::uuid)`

func (r *Repository) relations(ctx context.Context, me string, other *string) ([]Relation, error) {
	rows, err := r.pool.Query(ctx, relationsSQL, me, other)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Relation{}
	for rows.Next() {
		var x Relation
		if err := rows.Scan(&x.User.ID, &x.User.Username, &x.User.DisplayName, &x.User.About, &x.Status, &x.Since); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) List(ctx context.Context, me string) ([]Relation, error) {
	return r.relations(ctx, me, nil)
}

// Get returns the relationship with one user, or ErrNotFound when there is none.
func (r *Repository) Get(ctx context.Context, me, other string) (Relation, error) {
	rels, err := r.relations(ctx, me, &other)
	if err != nil {
		return Relation{}, err
	}
	if len(rels) == 0 {
		return Relation{}, ErrNotFound
	}
	// A pair can be both friends-row and blocked only transiently; blocked wins for display.
	for _, x := range rels {
		if x.Status == StatusBlocked {
			return x, nil
		}
	}
	return rels[0], nil
}

func (r *Repository) UserExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND deleted_at IS NULL AND status = 'active')`, id).Scan(&ok)
	return ok, err
}

func (r *Repository) IsBlocked(ctx context.Context, blocker, blocked string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM blocked_users WHERE blocker_id = $1 AND blocked_id = $2)`, blocker, blocked).Scan(&ok)
	return ok, err
}

func (r *Repository) DisplayName(ctx context.Context, id string) (string, error) {
	var n string
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(p.display_name, u.username) FROM users u LEFT JOIN user_profiles p ON p.user_id = u.id WHERE u.id = $1`, id).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return n, err
}

// Request = follow: creates a pending follow me → other. If other already follows me, this is a follow back and the pair becomes friends.
// created reports whether a NEW pending request was made (the caller notifies only then); accepted reports the mutual case.
// Any other existing row for the pair (already friends, or I already asked) makes this an idempotent no-op.
func (r *Repository) Request(ctx context.Context, me, other string) (created, accepted bool, err error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE friendships SET status = 'accepted', responded_at = now()
		WHERE requester_id = $2 AND addressee_id = $1 AND status = 'pending'`, me, other)
	if err != nil {
		return false, false, err
	}
	if tag.RowsAffected() == 1 {
		return false, true, nil
	}
	// ON CONFLICT DO NOTHING on the unordered-pair unique index: a concurrent request from the other side wins and ours is a no-op.
	tag, err = r.pool.Exec(ctx, `INSERT INTO friendships (requester_id, addressee_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, me, other)
	if err != nil {
		return false, false, err
	}
	return tag.RowsAffected() == 1, false, nil
}

func (r *Repository) Accept(ctx context.Context, me, requester string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE friendships SET status = 'accepted', responded_at = now()
		WHERE requester_id = $2 AND addressee_id = $1 AND status = 'pending'`, me, requester)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePending removes a pending request in either direction (decline if I am the addressee, cancel if I am the requester).
func (r *Repository) DeletePending(ctx context.Context, me, other string) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM friendships WHERE status = 'pending'
		AND ((requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1))`, me, other)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveFriend is "unfollow" for a mutual pair: I stop following them, but they keep following me, so the row
// becomes a pending follow other → me (I can follow back later, they see "Following").
func (r *Repository) RemoveFriend(ctx context.Context, me, other string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE friendships SET requester_id = $2, addressee_id = $1, status = 'pending', responded_at = NULL
		WHERE status = 'accepted'
		AND ((requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1))`, me, other)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Block records the block and drops any friendship/request between the pair in one transaction.
func (r *Repository) Block(ctx context.Context, me, other string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO blocked_users (blocker_id, blocked_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, me, other); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM friendships WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)`, me, other); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) Unblock(ctx context.Context, me, other string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM blocked_users WHERE blocker_id = $1 AND blocked_id = $2`, me, other)
	return err
}

// Notifier is satisfied by *inbox.Service.
type Notifier interface {
	Notify(ctx context.Context, userID, kind, title, body string, actorID, conversationID *string) error
}

type Service struct {
	repo   *Repository
	notify Notifier // may be nil
}

func NewService(repo *Repository, n Notifier) *Service { return &Service{repo: repo, notify: n} }

func checkID(me, other string) error {
	if _, err := uuid.Parse(other); err != nil {
		return ErrInvalidID
	}
	if me == other {
		return ErrSelf
	}
	return nil
}

func (s *Service) List(ctx context.Context, me string) ([]Relation, error) {
	return s.repo.List(ctx, me)
}

func (s *Service) tell(ctx context.Context, to, kind, title, body, actor string) {
	if s.notify != nil {
		_ = s.notify.Notify(ctx, to, kind, title, body, &actor, nil) // best-effort; the friendship change already succeeded
	}
}

func (s *Service) SendRequest(ctx context.Context, me, other string) (Relation, error) {
	if err := checkID(me, other); err != nil {
		return Relation{}, err
	}
	if ok, err := s.repo.UserExists(ctx, other); err != nil {
		return Relation{}, err
	} else if !ok {
		return Relation{}, ErrNotFound
	}
	if blocked, err := s.repo.IsBlocked(ctx, me, other); err != nil {
		return Relation{}, err
	} else if blocked {
		return Relation{}, ErrBlocked
	}
	// If THEY blocked me, behave exactly as if the user did not exist: do not reveal the block.
	if blockedByThem, err := s.repo.IsBlocked(ctx, other, me); err != nil {
		return Relation{}, err
	} else if blockedByThem {
		return Relation{}, ErrNotFound
	}
	created, accepted, err := s.repo.Request(ctx, me, other)
	if err != nil {
		return Relation{}, err
	}
	if created || accepted {
		name, _ := s.repo.DisplayName(ctx, me)
		if accepted {
			s.tell(ctx, other, "friend_accept", name+" followed you back", "You are now friends. You can chat and see each other's status.", me)
		} else {
			s.tell(ctx, other, "friend_request", name+" started following you", "Follow back to become friends.", me)
		}
	}
	return s.repo.Get(ctx, me, other)
}

func (s *Service) Accept(ctx context.Context, me, other string) (Relation, error) {
	if err := checkID(me, other); err != nil {
		return Relation{}, err
	}
	if err := s.repo.Accept(ctx, me, other); err != nil {
		return Relation{}, err
	}
	name, _ := s.repo.DisplayName(ctx, me)
	s.tell(ctx, other, "friend_accept", name+" followed you back", "You are now friends. You can chat and see each other's status.", me)
	return s.repo.Get(ctx, me, other)
}

func (s *Service) DeletePending(ctx context.Context, me, other string) error {
	if err := checkID(me, other); err != nil {
		return err
	}
	return s.repo.DeletePending(ctx, me, other)
}

func (s *Service) Remove(ctx context.Context, me, other string) error {
	if err := checkID(me, other); err != nil {
		return err
	}
	return s.repo.RemoveFriend(ctx, me, other)
}

func (s *Service) Block(ctx context.Context, me, other string) error {
	if err := checkID(me, other); err != nil {
		return err
	}
	if ok, err := s.repo.UserExists(ctx, other); err != nil {
		return err
	} else if !ok {
		return ErrNotFound
	}
	return s.repo.Block(ctx, me, other)
}

func (s *Service) Unblock(ctx context.Context, me, other string) error {
	if err := checkID(me, other); err != nil {
		return err
	}
	return s.repo.Unblock(ctx, me, other)
}
