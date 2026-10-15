-- 0012: email + password accounts, phone becomes optional and unverified.
-- OTP/phone login keeps working for existing accounts (no data is removed).

ALTER TABLE users ALTER COLUMN phone_number DROP NOT NULL;
ALTER TABLE users ALTER COLUMN country_code DROP NOT NULL;
ALTER TABLE users ALTER COLUMN phone_number_hash DROP NOT NULL;

ALTER TABLE users ADD COLUMN email TEXT;                 -- stored lower-cased
ALTER TABLE users ADD COLUMN password_hash TEXT;         -- argon2id, NULL for legacy OTP-only accounts
-- Optional contact number typed in by the user. NOT verified, so it is deliberately kept out of
-- phone_number / phone_number_hash: otherwise anyone could squat someone else's number, block that
-- person's real OTP sign-up, or hijack contact-sync matches. Display only; never used for lookup.
ALTER TABLE users ADD COLUMN contact_phone TEXT;
ALTER TABLE users ADD COLUMN email_verified_at TIMESTAMPTZ; -- NULL = unverified (verification not enforced yet)

CREATE UNIQUE INDEX uq_users_email ON users (email) WHERE deleted_at IS NULL AND email IS NOT NULL;
ALTER TABLE users ADD CONSTRAINT chk_users_email_lower CHECK (email IS NULL OR email = lower(email));
ALTER TABLE users ADD CONSTRAINT chk_users_has_identity CHECK (email IS NOT NULL OR phone_number IS NOT NULL);

CREATE TABLE password_reset_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL,                 -- SHA-256 hex; the raw token is only ever in the e-mailed link
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_password_reset_token_hash ON password_reset_tokens (token_hash);
CREATE INDEX idx_password_reset_user ON password_reset_tokens (user_id) WHERE used_at IS NULL;
