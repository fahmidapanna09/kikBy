package redisx

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// tlsEnabled is true for ElastiCache with transit encryption on (see
// terraform/elasticache.tf's replication group) and false for the local
// docker-compose Redis every prior phase has run against — plain TCP,
// no cert to manage, same as always. Flipping this on never changes
// anything else about how the client is used; go-redis handles TLS
// transparently once the connection is established.
func New(ctx context.Context, addr, password string, db int, tlsEnabled bool) (*redis.Client, error) {
	opts := &redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
	if tlsEnabled {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		return nil, fmt.Errorf("redisx: ping failed: %w", err)
	}
	return client, nil
}
