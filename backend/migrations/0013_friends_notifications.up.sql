-- 0013: friend requests (two-way, needs approval — distinct from the one-way phone "contacts" of 0005)
-- and an in-app notification inbox. Blocking keeps using blocked_users (0005) so chat/stories/contacts
-- all see one source of truth.

CREATE TABLE friendships (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    requester_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    addressee_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at  TIMESTAMPTZ,
    CONSTRAINT chk_friendship_not_self CHECK (requester_id <> addressee_id)
);
-- one row per unordered pair, whichever side asked first
CREATE UNIQUE INDEX uq_friendships_pair ON friendships (LEAST(requester_id, addressee_id), GREATEST(requester_id, addressee_id));
CREATE INDEX idx_friendships_addressee ON friendships (addressee_id, status);
CREATE INDEX idx_friendships_requester ON friendships (requester_id, status);

CREATE TABLE app_notifications (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind             TEXT NOT NULL CHECK (kind IN ('message', 'friend_request', 'friend_accept', 'mention', 'system')),
    title            TEXT NOT NULL,
    body             TEXT NOT NULL DEFAULT '',
    actor_id         UUID REFERENCES users(id) ON DELETE SET NULL,
    conversation_id  UUID,
    read_at          TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_app_notifications_user ON app_notifications (user_id, created_at DESC);
CREATE INDEX idx_app_notifications_unread ON app_notifications (user_id) WHERE read_at IS NULL;
