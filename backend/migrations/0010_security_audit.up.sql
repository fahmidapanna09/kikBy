-- 0010_security_audit.up.sql
-- Phase 10: security hardening. A append-only log of security-relevant
-- events (login, logout, session revocation, admin role changes, etc.)
-- for incident response and abuse investigation.
--
-- Hard rule, enforced by code review and internal/platform/audit's own
-- doc comment, not by this schema: `metadata` must NEVER contain an OTP
-- code, a password, a raw token, or message/post content. This table
-- exists to answer "who did what, from where, when" — never "what did
-- they say".

CREATE TABLE security_audit_log (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID REFERENCES users(id) ON DELETE SET NULL, -- nullable: e.g. a failed OTP verify for an unregistered phone has no user yet
    event_type TEXT NOT NULL,
    ip_address INET,
    user_agent TEXT,
    metadata   JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_security_audit_log_user ON security_audit_log (user_id, created_at DESC);
CREATE INDEX idx_security_audit_log_event_type ON security_audit_log (event_type, created_at DESC);
