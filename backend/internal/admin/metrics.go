package admin

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MetricsRepository is a deliberate, narrow exception to this project's
// "no module reaches into another module's tables directly" rule (stated
// in docs/00-architecture-and-roadmap.md section 1 and followed by every
// other cross-module interaction in this codebase via small Go
// interfaces). Reasoning for the exception, so it reads as a decision and
// not an oversight:
//
//  1. These are read-only COUNT/aggregate queries with zero business
//     logic — there is nothing for an interface to abstract over except
//     the SQL itself, so a UserDirectory-style interface per module would
//     be pure boilerplate (one near-identical "count my rows" method per
//     module) for a dashboard, not a meaningful seam.
//  2. In a real split to microservices, analytics/reporting is itself
//     typically its own service reading from a replica or a data
//     warehouse, not something that goes through every other service's
//     API one query at a time — so this code wouldn't become "the thing
//     to refactor" during that split; it'd be replaced wholesale.
//  3. It's isolated to exactly this one file, so the exception is easy to
//     find, re-evaluate, or delete later without touching admin's other
//     (properly interface-bound) functionality in service.go.
type MetricsRepository struct {
	pool *pgxpool.Pool
}

func NewMetricsRepository(pool *pgxpool.Pool) *MetricsRepository {
	return &MetricsRepository{pool: pool}
}

// Overview is a snapshot of current totals — good enough for a single
// dashboard screen's headline numbers, not a time-series (see GrowthSeries
// for that). Every query here is a single COUNT(*) against one table;
// none of it joins across modules or encodes any business rule.
func (m *MetricsRepository) Overview(ctx context.Context) (map[string]any, error) {
	queries := map[string]string{
		"total_users":            `SELECT count(*) FROM users WHERE deleted_at IS NULL`,
		"suspended_users":        `SELECT count(*) FROM users WHERE status = 'suspended' AND deleted_at IS NULL`,
		"active_sessions":        `SELECT count(*) FROM user_sessions WHERE revoked_at IS NULL AND expires_at > now()`,
		"messages_last_24h":      `SELECT count(*) FROM messages WHERE created_at > now() - interval '24 hours'`,
		"groups_total":           `SELECT count(*) FROM groups`,
		"communities_total":      `SELECT count(*) FROM communities`,
		"channels_total":         `SELECT count(*) FROM channels WHERE deleted_at IS NULL`,
		"stories_last_24h":       `SELECT count(*) FROM stories WHERE created_at > now() - interval '24 hours'`,
		"ai_conversations_total": `SELECT count(*) FROM ai_conversations WHERE deleted_at IS NULL`,
		"unresolved_reports":     `SELECT count(*) FROM channel_reports WHERE resolved_at IS NULL`,
	}

	out := make(map[string]any, len(queries))
	for key, q := range queries {
		var n int64
		if err := m.pool.QueryRow(ctx, q).Scan(&n); err != nil {
			return nil, err
		}
		out[key] = n
	}
	return out, nil
}

// GrowthSeries returns one row per day for the last `days` days: how many
// new user accounts were created that day. A minimal "dashboard" time
// series — see backend/README.md's Phase 11 known gaps for what a real
// metrics/observability stack (Prometheus histograms, Grafana, etc.)
// would add on top of this.
func (m *MetricsRepository) GrowthSeries(ctx context.Context, days int) ([]map[string]any, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT date_trunc('day', d)::date AS day,
		       COALESCE((SELECT count(*) FROM users u WHERE date_trunc('day', u.created_at) = date_trunc('day', d) AND u.deleted_at IS NULL), 0) AS new_users
		FROM generate_series(now() - ($1 || ' days')::interval, now(), interval '1 day') AS d
		ORDER BY day ASC
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var day time.Time
		var newUsers int64
		if err := rows.Scan(&day, &newUsers); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"day": day.Format("2006-01-02"), "new_users": newUsers})
	}
	return out, rows.Err()
}
