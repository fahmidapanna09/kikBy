-- 0018: per-user client settings (privacy / notification / appearance / chat toggles) synced across devices.
-- Stored as an opaque JSON object; the server validates shape/size only. Enforcement of privacy toggles
-- (e.g. read receipts) is separate from storing the user's choice.
CREATE TABLE IF NOT EXISTS user_settings (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    data       JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
