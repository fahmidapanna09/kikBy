package auth

import (
	"context"
	"time"
)

// CountRecentFailedLogins counts login.failed events for the account inside the window, ignoring anything before the
// last successful sign-in or completed password reset (either one clears the slate).
func (r *Repository) CountRecentFailedLogins(ctx context.Context, userID string, window time.Duration) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM security_audit_log
		WHERE user_id = $1 AND event_type = 'login.failed'
		  AND created_at > now() - make_interval(secs => $2)
		  AND created_at > COALESCE((SELECT MAX(created_at) FROM security_audit_log
		                             WHERE user_id = $1 AND event_type IN ('login.succeeded', 'password.reset_completed')), '-infinity')`,
		userID, window.Seconds()).Scan(&n)
	return n, err
}
