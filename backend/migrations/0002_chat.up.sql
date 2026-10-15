-- 0002_chat.up.sql
-- Phase 2: 1:1 conversations, messages, delivery/read receipts, push tokens.
-- Group-specific columns/tables (groups, group_members, etc.) land in the
-- Phase 4 migration; conversations.type already anticipates 'group' so that
-- migration only adds tables, it doesn't touch this one.

CREATE TABLE conversations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type            TEXT NOT NULL DEFAULT 'direct', -- direct | group | channel (group/channel used from Phase 4/8)
    -- for type='direct' only: canonical ordered pair so we can upsert
    -- idempotently instead of racing two inserts for the same two users.
    direct_key      TEXT UNIQUE,
    last_message_id UUID,             -- denormalized for fast conversation-list rendering
    last_message_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE conversation_members (
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_read_seq   BIGINT NOT NULL DEFAULT 0,   -- for unread-count computation
    muted_until     TIMESTAMPTZ,
    left_at         TIMESTAMPTZ,
    PRIMARY KEY (conversation_id, user_id)
);

CREATE INDEX idx_conversation_members_user ON conversation_members (user_id) WHERE left_at IS NULL;

-- seq is assigned per-conversation (see repository.go InsertMessage) and is
-- the authoritative ordering — never rely on created_at/client clocks for
-- ordering, only for display.
CREATE TABLE messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    seq             BIGINT NOT NULL,
    sender_id       UUID NOT NULL REFERENCES users(id),
    client_tmp_id   TEXT NOT NULL,   -- client-generated idempotency key
    type            TEXT NOT NULL DEFAULT 'text', -- text|image|video|audio|voice|document|location|contact_card|...
    body            TEXT,            -- text content; NULL for pure-media messages
    reply_to_id     UUID REFERENCES messages(id),
    edited_at       TIMESTAMPTZ,
    deleted_for_everyone_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_messages_conversation_seq ON messages (conversation_id, seq);
-- idempotency: the same sender retrying the same client_tmp_id in the same
-- conversation must not create a duplicate row.
CREATE UNIQUE INDEX uq_messages_conversation_sender_tmpid ON messages (conversation_id, sender_id, client_tmp_id);
CREATE INDEX idx_messages_conversation_created ON messages (conversation_id, created_at DESC);

-- per-recipient delete-for-me; deleted_for_everyone is a column on messages
-- itself since it applies to all recipients at once.
CREATE TABLE message_deletions (
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deleted_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, user_id)
);

CREATE TABLE message_deliveries (
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    delivered_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, user_id)
);

CREATE TABLE message_reads (
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    read_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, user_id)
);

-- FCM registration tokens, separate from user_devices (a device can rotate
-- its push token without a new device row).
CREATE TABLE push_tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       UUID NOT NULL REFERENCES user_devices(id) ON DELETE CASCADE,
    fcm_token       TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_push_tokens_device ON push_tokens (device_id);
CREATE INDEX idx_push_tokens_user ON push_tokens (user_id);

-- Durable outbox for push notification delivery, processed by the worker.
-- Keeping this as a table (not only a Redis queue) means a crashed worker
-- never silently drops a push — it just retries the row.
CREATE TABLE notification_outbox (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,   -- message.new | call.incoming | ...
    payload         JSONB NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending', -- pending | sent | failed
    attempts        INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at         TIMESTAMPTZ
);

CREATE INDEX idx_notification_outbox_pending ON notification_outbox (created_at) WHERE status = 'pending';

CREATE TRIGGER trg_conversations_updated_at BEFORE UPDATE ON conversations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
