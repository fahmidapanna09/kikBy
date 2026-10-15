-- 0020: voice / video / group calls (WebRTC signaling state + history).
-- Media never touches the server: peers connect directly (mesh), the server only relays offers/answers/ICE.
CREATE TABLE IF NOT EXISTS calls (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    initiator_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('voice', 'video')),
    -- ringing: waiting for someone to answer · active: at least one other person joined
    -- ended: normal · missed: nobody answered in time · declined: 1:1 callee rejected · cancelled: caller hung up while ringing
    status          TEXT NOT NULL DEFAULT 'ringing' CHECK (status IN ('ringing', 'active', 'ended', 'missed', 'declined', 'cancelled')),
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    answered_at     TIMESTAMPTZ,
    ended_at        TIMESTAMPTZ,
    end_reason      TEXT
);
CREATE INDEX IF NOT EXISTS calls_conversation_started_idx ON calls (conversation_id, started_at DESC);
-- At most one live call per conversation: a second "start" becomes a "join".
CREATE UNIQUE INDEX IF NOT EXISTS calls_one_live_per_conversation ON calls (conversation_id) WHERE status IN ('ringing', 'active');

CREATE TABLE IF NOT EXISTS call_participants (
    call_id   UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at   TIMESTAMPTZ,
    PRIMARY KEY (call_id, user_id)
);
CREATE INDEX IF NOT EXISTS call_participants_user_idx ON call_participants (user_id) WHERE left_at IS NULL;
