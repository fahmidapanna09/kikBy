// Package audit writes to security_audit_log — a "who did what, from
// where, when" trail for incident response, distinct from the application
// logs (zap) and from message/content storage.
//
// Hard rule: metadata passed to Log must NEVER contain an OTP code, a
// password, a raw access/refresh token, or message/post/story content.
// This mirrors the same "never log secrets" rule the spec applies to
// structured logs (see internal/platform/securitylint for a test that
// greps for violations of that rule in zap calls) — this package is the
// other place the same discipline matters, because a DB row is just as
// permanent as a log line, arguably more so.
package audit

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type EventType string

// Phase 10 wires these into the auth module only (login, logout, session
// revocation, refresh-token reuse detection) — the highest-value surface
// for "who did what, from where, when". Extending this same Logger into
// channels/communities admin-role-change events (grant/revoke admin) is a
// natural next step, deliberately left as a follow-up rather than done
// here — see backend/README.md's Phase 10 known gaps.
const (
	EventOTPRequested           EventType = "otp.requested"
	EventOTPVerifyFailed        EventType = "otp.verify_failed"
	EventLoginSucceeded         EventType = "login.succeeded"
	EventLoginFailed            EventType = "login.failed"
	EventRegistered             EventType = "account.registered"
	EventPasswordResetRequested EventType = "password.reset_requested"
	EventPasswordResetCompleted EventType = "password.reset_completed"
	EventEmailVerified          EventType = "email.verified"
	EventPasswordChanged        EventType = "password.changed"
	EventEmailChangeRequested   EventType = "email.change_requested"
	EventAccountDeleted         EventType = "account.deleted"
	EventLoginBlockedSuspend    EventType = "login.blocked_suspended"
	EventLogoutSucceeded        EventType = "logout.succeeded"
	EventSessionRevoked         EventType = "session.revoked"
	EventAllSessionsRevoked     EventType = "session.revoked_all"
	EventRefreshTokenReused     EventType = "refresh_token.reuse_detected"

	// Phase 11 (admin module) events. Defined here, not in internal/admin,
	// so both auth (which performs the suspend/unsuspend) and admin (which
	// grants/revokes roles) can log through the one shared audit.Logger
	// without either module importing the other.
	EventUserSuspended    EventType = "admin.user_suspended"
	EventUserUnsuspended  EventType = "admin.user_unsuspended"
	EventAdminRoleGranted EventType = "admin.role_granted"
	EventAdminRoleRevoked EventType = "admin.role_revoked"
	EventReportResolved   EventType = "admin.report_resolved"
)

// Logger is safe to use with a nil receiver (Log becomes a no-op) so call
// sites and tests never need a real Postgres pool just to construct a
// Service — see auth.NewService, which accepts *audit.Logger directly.
type Logger struct {
	pool *pgxpool.Pool
	log  *zap.Logger
}

func New(pool *pgxpool.Pool, log *zap.Logger) *Logger {
	return &Logger{pool: pool, log: log}
}

// Log records one event. userID, ipAddress, and userAgent may all be empty
// strings (stored as NULL) when not applicable or not yet known. Write
// failures are logged as a warning and otherwise swallowed — an audit
// write must never fail the real request it's describing.
func (l *Logger) Log(ctx context.Context, eventType EventType, userID, ipAddress, userAgent string, metadata map[string]any) {
	if l == nil || l.pool == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		metaJSON = []byte("{}")
	}

	var uid *string
	if userID != "" {
		uid = &userID
	}

	_, err = l.pool.Exec(ctx, `
		INSERT INTO security_audit_log (user_id, event_type, ip_address, user_agent, metadata)
		VALUES ($1, $2, NULLIF($3, '')::inet, NULLIF($4, ''), $5)
	`, uid, string(eventType), ipAddress, userAgent, metaJSON)
	if err != nil && l.log != nil {
		l.log.Warn("security audit log write failed", zap.Error(err), zap.String("event_type", string(eventType)))
	}
}
