-- 0014_chat_web.up.sql
-- Per-member pin/archive state for the web client's conversation list.
-- Both are per user (a pin in my list says nothing about yours), so they live
-- on conversation_members next to muted_until, not on conversations.
ALTER TABLE conversation_members
    ADD COLUMN IF NOT EXISTS pinned_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

