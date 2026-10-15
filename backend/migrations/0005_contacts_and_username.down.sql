DROP TRIGGER IF EXISTS trg_contacts_updated_at ON contacts;

DROP TABLE IF EXISTS reported_users;
DROP TABLE IF EXISTS blocked_users;
DROP TABLE IF EXISTS contacts;

ALTER TABLE users ALTER COLUMN username DROP NOT NULL;

DROP INDEX IF EXISTS uq_users_phone_hash;
ALTER TABLE users DROP COLUMN IF EXISTS phone_number_hash;
