-- 0008_channels.up.sql
-- Phase 8: Channels — one-way broadcast. Distinct from groups/communities:
-- channel_posts is its own table, not routed through `messages` at all,
-- because channels have no delivery receipts, typing, or two-way reply —
-- just posts, reactions, and view counts. Polls are modeled generically
-- enough here that the same tables could back group polls later (deferred
-- in Phase 4) without a schema change.

CREATE TABLE channels (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    description   TEXT,
    icon_media_id UUID REFERENCES media_files(id),
    category      TEXT,                              -- free-text for now; see README for why not a lookup table
    is_public     BOOLEAN NOT NULL DEFAULT true,       -- false = invite-link/code only, not in search
    invite_code   TEXT UNIQUE,
    created_by    UUID NOT NULL REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);

CREATE INDEX idx_channels_public_name ON channels (name) WHERE is_public AND deleted_at IS NULL;
CREATE INDEX idx_channels_invite_code ON channels (invite_code) WHERE invite_code IS NOT NULL;

CREATE TABLE channel_admins (
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'admin', -- owner | admin
    added_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, user_id)
);

-- Followers are NOT admins — one-way by construction: being a follower
-- grants read/react/view, never post. Admin status is entirely separate
-- (an admin doesn't need to also "follow" to post, but may).
CREATE TABLE channel_followers (
    channel_id  UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    muted       BOOLEAN NOT NULL DEFAULT false,
    followed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, user_id)
);
CREATE INDEX idx_channel_followers_user ON channel_followers (user_id);

CREATE TABLE channel_posts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id   UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    author_id    UUID NOT NULL REFERENCES users(id),     -- must have been an admin at post time
    seq          BIGINT NOT NULL,                        -- per-channel monotonic order, same pattern as messages.seq
    post_type    TEXT NOT NULL,                           -- text | image | video | audio | sticker | poll | link
    body         TEXT,
    media_id     UUID REFERENCES media_files(id),
    link_url     TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_at    TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_channel_posts_channel_seq ON channel_posts (channel_id, seq);
CREATE INDEX idx_channel_posts_channel_created ON channel_posts (channel_id, created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE channel_post_reactions (
    post_id    UUID NOT NULL REFERENCES channel_posts(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, user_id)
);

CREATE TABLE channel_post_views (
    post_id   UUID NOT NULL REFERENCES channel_posts(id) ON DELETE CASCADE,
    user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    viewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, user_id)
);

-- --- Polls (poll-type posts only) ---

CREATE TABLE channel_poll_options (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id  UUID NOT NULL REFERENCES channel_posts(id) ON DELETE CASCADE,
    text     TEXT NOT NULL,
    position INT NOT NULL DEFAULT 0
);
CREATE INDEX idx_channel_poll_options_post ON channel_poll_options (post_id, position);

CREATE TABLE channel_poll_votes (
    option_id UUID NOT NULL REFERENCES channel_poll_options(id) ON DELETE CASCADE,
    user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    voted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (option_id, user_id)
);
CREATE INDEX idx_channel_poll_votes_user ON channel_poll_votes (user_id);

CREATE TABLE channel_reports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id  UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    reason      TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_channels_updated_at BEFORE UPDATE ON channels
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
