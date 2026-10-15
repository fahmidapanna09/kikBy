-- 0021: e-mail verification switches from an e-mailed link to a 6-digit code (valid 10 minutes).
-- token_hash now holds a salted Argon2id hash of the code ("salt:hash"), never the code itself.
-- attempts counts wrong guesses against the newest open code; after 5 the code is burned and a new one must be requested.
ALTER TABLE email_verification_tokens ADD COLUMN attempts SMALLINT NOT NULL DEFAULT 0;

-- Links that were already e-mailed can no longer be redeemed by anyone: close every open row.
UPDATE email_verification_tokens SET used_at = now() WHERE used_at IS NULL;
