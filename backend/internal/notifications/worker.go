package notifications

import (
	"context"
	"time"

	"go.uber.org/zap"
)

type Worker struct {
	outbox *Outbox
	tokens *TokenRepository
	sender PushSender
	log    *zap.Logger

	pollInterval time.Duration
	batchSize    int
}

func NewWorker(outbox *Outbox, tokens *TokenRepository, sender PushSender, log *zap.Logger) *Worker {
	return &Worker{
		outbox:       outbox,
		tokens:       tokens,
		sender:       sender,
		log:          log,
		pollInterval: 2 * time.Second,
		batchSize:    50,
	}
}

// Run polls until ctx is cancelled. Intended to run as its own process
// (cmd/worker) so notification delivery load never competes with the API's
// request-handling goroutines, and so it can be scaled independently.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *Worker) processBatch(ctx context.Context) {
	items, err := w.outbox.ClaimBatch(ctx, w.batchSize)
	if err != nil {
		w.log.Error("worker: claim batch failed", zap.Error(err))
		return
	}

	for _, item := range items {
		tokens, err := w.tokens.TokensForUser(ctx, item.UserID)
		if err != nil {
			w.log.Error("worker: failed to load tokens", zap.Error(err), zap.String("outbox_id", item.ID))
			_ = w.outbox.MarkFailed(ctx, item.ID, item.Attempts)
			continue
		}
		if len(tokens) == 0 {
			// No device to push to (e.g. never registered, or all revoked).
			// Not a failure — nothing more to do with this item.
			_ = w.outbox.MarkSent(ctx, item.ID)
			continue
		}

		var lastErr error
		for _, tok := range tokens {
			if err := w.sender.Send(ctx, tok, item.Kind, item.Payload); err != nil {
				lastErr = err
			}
		}

		if lastErr != nil {
			w.log.Warn("worker: push send failed, will retry", zap.Error(lastErr), zap.String("outbox_id", item.ID), zap.Int("attempts", item.Attempts))
			_ = w.outbox.MarkFailed(ctx, item.ID, item.Attempts)
		} else {
			_ = w.outbox.MarkSent(ctx, item.ID)
		}
	}
}
