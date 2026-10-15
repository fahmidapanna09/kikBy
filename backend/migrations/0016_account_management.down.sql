ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_pending_email_lower;
ALTER TABLE users DROP COLUMN IF EXISTS pending_email;
