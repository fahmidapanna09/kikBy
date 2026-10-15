package notifications

import (
	"context"

	"go.uber.org/zap"
)

// PushSender abstracts Firebase Cloud Messaging so it can be mocked in
// tests and swapped for a real implementation (firebase.google.com/go/v4
// messaging client) without touching the outbox worker.
type PushSender interface {
	// Send delivers a data-only or notification+data push to a specific FCM
	// registration token. Returns an error the worker will retry on, except
	// for ErrInvalidToken which means the token is dead and should be
	// dropped rather than retried.
	Send(ctx context.Context, fcmToken, kind string, payload []byte) error
}

// LoggingStubSender is the Phase 2 default: it logs what *would* be sent
// instead of calling real FCM, so the outbox/worker pipeline is fully
// testable before Firebase credentials exist. Swap for a real
// firebase-admin-backed sender when FCM credentials are provisioned.
type LoggingStubSender struct {
	Log *zap.Logger
}

func (s LoggingStubSender) Send(ctx context.Context, fcmToken, kind string, payload []byte) error {
	s.Log.Info("push (stub, not actually sent)",
		zap.String("kind", kind),
		zap.Int("payload_bytes", len(payload)),
		zap.String("token_suffix", lastN(fcmToken, 6)), // never log the full token
	)
	return nil
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
