-- 0007_communities.up.sql
-- Phase 7: Communities — a container for multiple existing groups, plus one
-- special "announcement group" every member is auto-joined to (read-only
-- for non-admins, reusing groups.who_can_send rather than inventing a
-- second permission system).

CREATE TABLE communities (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT NOT NULL,
    description    TEXT,
    icon_media_id  UUID REFERENCES media_files(id),
    invite_code    TEXT UNIQUE,
    discoverable   BOOLEAN NOT NULL DEFAULT false, -- false = joinable only via invite link; true = shows up in community search
    created_by     UUID NOT NULL REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_communities_invite_code ON communities (invite_code) WHERE invite_code IS NOT NULL;
CREATE INDEX idx_communities_discoverable_name ON communities (name) WHERE discoverable;

CREATE TABLE community_members (
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         TEXT NOT NULL DEFAULT 'member', -- owner | admin | member
    joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (community_id, user_id)
);

-- A ban is a member record's absence PLUS an entry here that blocks
-- rejoining — kept as its own table rather than a flag on community_members
-- so a plain "leave" never has to be distinguished from a "ban" in that table.
CREATE TABLE community_banned_users (
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    banned_by    UUID NOT NULL REFERENCES users(id),
    banned_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (community_id, user_id)
);

-- References groups(conversation_id) specifically (not conversations(id))
-- so only an actual group can be linked into a community, never a 1:1 chat.
CREATE TABLE community_groups (
    community_id    UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    conversation_id UUID NOT NULL REFERENCES groups(conversation_id) ON DELETE CASCADE,
    is_announcement BOOLEAN NOT NULL DEFAULT false,
    added_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (community_id, conversation_id)
);

-- At most one announcement group per community.
CREATE UNIQUE INDEX uq_community_groups_one_announcement ON community_groups (community_id) WHERE is_announcement;

CREATE TABLE community_reports (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    reason       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_communities_updated_at BEFORE UPDATE ON communities
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
