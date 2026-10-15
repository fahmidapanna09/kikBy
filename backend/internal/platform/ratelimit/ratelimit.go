// Package ratelimit provides a simple fixed-window rate limiter backed by
// Redis, shared by any module that needs to throttle an action per key
// (per-phone, per-IP, per-user). Not a sliding window — good enough for
// abuse control, not for billing-grade precision.
package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Limiter {
	return &Limiter{rdb: rdb}
}

// Allow increments the counter for key and reports whether it's still under
// max for the current window. The window starts on the first increment and
// resets `window` after that (a rolling reset point per key, not aligned to
// a global clock boundary — irrelevant for abuse-control purposes).
func (l *Limiter) Allow(ctx context.Context, key string, max int, window time.Duration) (bool, error) {
	count, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		l.rdb.Expire(ctx, key, window)
	}
	return count <= int64(max), nil
}
