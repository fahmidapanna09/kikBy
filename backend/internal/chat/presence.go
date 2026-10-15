package chat

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Presence tracks who is online across ALL backend nodes (not just this
// one), using a Redis key per user with a short TTL that connection
// heartbeats keep refreshing. This is deliberately separate from Hub's
// local connsByUser map, which only knows about connections on this node.
type Presence struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewPresence(rdb *redis.Client) *Presence {
	return &Presence{rdb: rdb, ttl: pongWait + 10*time.Second}
}

func onlineKey(userID string) string   { return "presence:online:" + userID }
func lastSeenKey(userID string) string { return "presence:last_seen:" + userID }

// MarkOnline is called on connect and refreshed on every pong. TTL means a
// crashed connection (that never sent a clean disconnect) still expires
// itself out of "online" within ~90s instead of staying stuck online forever.
func (p *Presence) MarkOnline(ctx context.Context, userID string) error {
	return p.rdb.Set(ctx, onlineKey(userID), "1", p.ttl).Err()
}

func (p *Presence) MarkOffline(ctx context.Context, userID string) error {
	if err := p.rdb.Del(ctx, onlineKey(userID)).Err(); err != nil {
		return err
	}
	return p.rdb.Set(ctx, lastSeenKey(userID), time.Now().UTC().Format(time.RFC3339), 30*24*time.Hour).Err()
}

func (p *Presence) IsOnline(ctx context.Context, userID string) (bool, error) {
	n, err := p.rdb.Exists(ctx, onlineKey(userID)).Result()
	return n > 0, err
}

func (p *Presence) LastSeen(ctx context.Context, userID string) (*time.Time, error) {
	val, err := p.rdb.Get(ctx, lastSeenKey(userID)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
