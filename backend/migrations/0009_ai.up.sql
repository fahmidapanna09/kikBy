-- 0009_ai.up.sql
-- Phase 9: Gemini AI Agent. Deliberately its own isolated module: an
-- ai_conversation is NOT a `conversations` row and ai_messages are NOT
-- `messages` rows. The AI agent has zero schema-level or Go-level access
-- to chat/groups/communities/channels data — see internal/ai/service.go
-- for why ("no auto-access to private chats" is a hard architectural rule,
-- not just a prompt instruction).

CREATE TABLE ai_conversations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT NOT NULL DEFAULT 'New chat',
    model      TEXT NOT NULL,              -- Gemini model name used for this conversation, recorded per-conversation
                                            -- so switching the server default later doesn't rewrite history
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_ai_conversations_user ON ai_conversations (user_id, updated_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE ai_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('user', 'model')),
    content         TEXT NOT NULL,
    finish_reason   TEXT,                  -- Gemini's finishReason for 'model' rows (e.g. STOP, SAFETY, MAX_TOKENS); null for 'user' rows
    error           TEXT,                  -- set when a 'model' row represents a failed generation instead of real content
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_messages_conversation ON ai_messages (conversation_id, created_at ASC);
