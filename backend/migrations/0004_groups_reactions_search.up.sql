-- 0004_groups_reactions_search.up.sql
-- Phase 4: groups (built on the existing conversations/conversation_members
-- tables from Phase 2, not a parallel set of tables), message reactions,
-- forward provenance, and Postgres full-text search over message bodies.

-- Meaningful for type='group' conversations; direct (1:1) conversations
-- leave both members at the default 'member' role since roles don't apply there.
ALTER TABLE conversation_members ADD COLUMN role TEXT NOT NULL DEFAULT 'member'; -- owner | admin | member

CREATE TABLE groups (
    conversation_id      UUID PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
    name                 TEXT NOT NULL,
    description          TEXT,
    icon_media_id        UUID REFERENCES media_files(id),
    who_can_send         TEXT NOT NULL DEFAULT 'everyone',      -- everyone | admins_only
    who_can_add_members  TEXT NOT NULL DEFAULT 'everyone',      -- everyone | admins_only
    who_can_edit_info    TEXT NOT NULL DEFAULT 'admins_only',   -- everyone | admins_only
    invite_code          TEXT UNIQUE,                           -- NULL = no active invite link
    created_by           UUID NOT NULL REFERENCES users(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_groups_invite_code ON groups (invite_code) WHERE invite_code IS NOT NULL;

CREATE TABLE message_reactions (
    message_id  UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, user_id) -- one reaction slot per user per message; setting a new emoji replaces the old one
);

-- Forwarding: a lightweight provenance pointer, not a full copy-tracking
-- table. The forwarded message is a normal, independently-persisted row;
-- this column only drives the "Forwarded" label client-side.
ALTER TABLE messages ADD COLUMN forwarded_from_message_id UUID REFERENCES messages(id);

-- Full-text search over message bodies. GENERATED STORED so it's always in
-- sync with `body` without application code having to remember to update it.
ALTER TABLE messages ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(body, ''))) STORED;

CREATE INDEX idx_messages_search_vector ON messages USING GIN (search_vector);

CREATE TRIGGER trg_groups_updated_at BEFORE UPDATE ON groups
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
