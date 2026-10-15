-- 0005_contacts_and_username.up.sql
-- Adds: a hashed phone number column (so contact sync never needs to see
-- other users' raw phone numbers), guarantees every user has a username
-- (backfilling existing rows), and the contacts/blocking/reporting tables.

-- ---- Phone number hashing ----
-- The client hashes each device-contact's phone number (SHA-256) before
-- ever sending it to the server for contact-sync matching; the server
-- matches against this column instead of ever seeing a third party's
-- plaintext number just because it's in someone's phone book.
ALTER TABLE users ADD COLUMN phone_number_hash TEXT;
UPDATE users SET phone_number_hash = encode(digest(phone_number, 'sha256'), 'hex') WHERE phone_number_hash IS NULL;
ALTER TABLE users ALTER COLUMN phone_number_hash SET NOT NULL;
CREATE UNIQUE INDEX uq_users_phone_hash ON users (phone_number_hash);

-- ---- Guarantee every account has a username ----
-- New accounts get one auto-generated at signup from here on (see
-- internal/auth/username.go); this backfills any pre-existing rows from
-- earlier phases' test data so the NOT NULL constraint can be added safely.
UPDATE users SET username = 'user_' || encode(gen_random_bytes(5), 'hex') WHERE username IS NULL;
ALTER TABLE users ALTER COLUMN username SET NOT NULL;
-- uq_users_username (from migration 0001) already covers uniqueness case-insensitively.

-- ---- Contacts ----
-- One-directional, WhatsApp-style: adding someone doesn't require their
-- approval and doesn't create a conversation by itself. contact_user_id is
-- NULL until/unless the phone number matches a registered account.
CREATE TABLE contacts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    phone_number    TEXT NOT NULL,   -- the owner's own data about their own contact; not a privacy concern for the owner
    nickname        TEXT,
    added_via       TEXT NOT NULL DEFAULT 'manual', -- manual | sync
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_contacts_owner_phone ON contacts (owner_id, phone_number);
CREATE INDEX idx_contacts_owner ON contacts (owner_id);
CREATE INDEX idx_contacts_contact_user ON contacts (contact_user_id) WHERE contact_user_id IS NOT NULL;

CREATE TABLE blocked_users (
    blocker_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id)
);

CREATE TABLE reported_users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reported_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason       TEXT NOT NULL,
    context_message_id UUID REFERENCES messages(id), -- optional: "report this specific message"
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_reported_users_reported ON reported_users (reported_id);

CREATE TRIGGER trg_contacts_updated_at BEFORE UPDATE ON contacts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
