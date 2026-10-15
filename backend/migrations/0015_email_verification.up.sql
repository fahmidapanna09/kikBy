-- 0015: e-mail address verification. Not enforced at login yet (users.email_verified_at already exists since 0012).
CREATE TABLE email_verification_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,                 -- the exact address this token proves; a later e-mail change must not inherit it
    token_hash  TEXT NOT NULL,                 -- SHA-256 hex; the raw token only ever exists in the e-mailed link
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_email_verification_token_hash ON email_verification_tokens (token_hash);
CREATE INDEX idx_email_verification_user ON email_verification_tokens (user_id, created_at DESC);
