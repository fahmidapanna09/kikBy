DROP TABLE IF EXISTS password_reset_tokens;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_has_identity;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_email_lower;
DROP INDEX IF EXISTS uq_users_email;
ALTER TABLE users DROP COLUMN IF EXISTS email_verified_at;
ALTER TABLE users DROP COLUMN IF EXISTS contact_phone;
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
ALTER TABLE users DROP COLUMN IF EXISTS email;
-- NOT NULL on phone columns is only restorable if no email-only users exist.
DELETE FROM users WHERE phone_number IS NULL;
ALTER TABLE users ALTER COLUMN phone_number_hash SET NOT NULL;
ALTER TABLE users ALTER COLUMN country_code SET NOT NULL;
ALTER TABLE users ALTER COLUMN phone_number SET NOT NULL;
