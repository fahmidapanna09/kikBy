package stories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// CleanupWorker hard-deletes stories a good while after they expire (default
// 30 days later, i.e. the length of a typical "recently deleted" grace
// window) and the media object each one points at. Soft-deleted stories
// (SoftDelete) are swept the same way, on the same delay — deliberately not
// immediate, so "delete story" has an undo window at the data layer even
// though no UI for that exists yet.
type CleanupWorker struct {
	pool         *pgxpool.Pool
	mediaDeleter MediaDeleter
	log          *zap.Logger
	pollInterval time.Duration
	retention    time.Duration
}

// MediaDeleter is satisfied structurally by media.Service (or a thin
// wrapper around it) — keeps this worker from importing the media package
// for anything beyond this one call.
type MediaDeleter interface {
	DeleteMedia(ctx context.Context, mediaID string) error
}

func NewCleanupWorker(pool *pgxpool.Pool, mediaDeleter MediaDeleter, log *zap.Logger) *CleanupWorker {
	return &CleanupWorker{pool: pool, mediaDeleter: mediaDeleter, log: log, pollInterval: time.Hour, retention: 30 * 24 * time.Hour}
}

func (w *CleanupWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	w.sweep(ctx) // once at startup, don't wait a full interval the first time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.sweep(ctx)
		}
	}
}

func (w *CleanupWorker) sweep(ctx context.Context) {
	cutoff := time.Now().Add(-w.retention)

	rows, err := w.pool.Query(ctx, `
		SELECT id, media_id FROM stories
		WHERE (expires_at < $1 OR deleted_at < $1)
		LIMIT 200
	`, cutoff)
	if err != nil {
		w.log.Error("stories cleanup: query failed", zap.Error(err))
		return
	}
	type row struct {
		id      string
		mediaID *string
	}
	var toDelete []row
	for rows.Next() {
		var rrow row
		if err := rows.Scan(&rrow.id, &rrow.mediaID); err != nil {
			w.log.Error("stories cleanup: scan failed", zap.Error(err))
			rows.Close()
			return
		}
		toDelete = append(toDelete, rrow)
	}
	rows.Close()

	for _, rrow := range toDelete {
		if rrow.mediaID != nil {
			if err := w.mediaDeleter.DeleteMedia(ctx, *rrow.mediaID); err != nil {
				w.log.Warn("stories cleanup: failed to delete media, will retry next sweep", zap.Error(err), zap.String("story_id", rrow.id))
				continue // leave the story row so we retry the whole thing next time
			}
		}
		if _, err := w.pool.Exec(ctx, `DELETE FROM stories WHERE id = $1`, rrow.id); err != nil {
			w.log.Warn("stories cleanup: failed to delete story row", zap.Error(err), zap.String("story_id", rrow.id))
		}
	}
	if len(toDelete) > 0 {
		w.log.Info("stories cleanup: swept old stories", zap.Int("count", len(toDelete)))
	}
}
