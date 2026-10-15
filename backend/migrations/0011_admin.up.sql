-- 0011_admin.up.sql
-- Phase 11: admin panel (RBAC) + observability support.

-- One role per user. A user with no row here is not an admin at all (the
-- common case — this is not the `users.status` column, which every
-- regular account has).
CREATE TABLE admin_roles (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('moderator', 'admin', 'superadmin')),
    granted_by UUID REFERENCES users(id),
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- `users.status` already existed since Phase 1 (active|suspended|deleted)
-- but nothing ever set or checked it. These three columns add the "why
-- and by whom" an account was suspended, for the admin UI and audit
-- trail; `status` itself remains the one column the login flow actually
-- branches on (internal/auth/service.go VerifyOTP).
ALTER TABLE users
    ADD COLUMN suspended_reason TEXT,
    ADD COLUMN suspended_at     TIMESTAMPTZ,
    ADD COLUMN suspended_by     UUID REFERENCES users(id);

-- Extends channel_reports (channels module, Phase 8), which was
-- write-only until now (reports went in, nobody could mark them done).
-- Living in this migration file rather than a new 0008-adjacent one
-- because it's Phase 11's admin workflow that needs it — the Go-level
-- module boundary (channels.Repository remains the only code that
-- queries this table) is unaffected by which migration file a column
-- was added in.
ALTER TABLE channel_reports
    ADD COLUMN resolved_at     TIMESTAMPTZ,
    ADD COLUMN resolved_by     UUID REFERENCES users(id),
    ADD COLUMN resolution_note TEXT;

CREATE INDEX idx_admin_roles_role ON admin_roles (role);
CREATE INDEX idx_channel_reports_unresolved ON channel_reports (created_at) WHERE resolved_at IS NULL;
