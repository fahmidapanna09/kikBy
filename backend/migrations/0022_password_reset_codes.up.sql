-- 0022: password reset switches from an e-mailed link to a 6-digit code (valid 10 minutes), like e-mail verification.
-- token_hash now holds a salted Argon2id hash of the code ("salt:hash"); attempts counts wrong guesses (5 burn the code).
ALTER TABLE password_reset_tokens ADD COLUMN attempts SMALLINT NOT NULL DEFAULT 0;

-- Reset links that were already e-mailed can no longer be redeemed: close every open row.
UPDATE password_reset_tokens SET used_at = now() WHERE used_at IS NULL;
