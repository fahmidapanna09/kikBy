-- 0006_stories.up.sql
-- Phase 6: 24-hour Stories/Status. Media lives in object storage (media_files);
-- a story row only references it. Expiry is enforced by queries filtering on
-- expires_at, so a story disappears from every feed the instant it expires
-- without needing a background job to "delete" it; expired-but-not-deleted
-- rows are the owner's archive.

CREATE TABLE stories (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_type       TEXT NOT NULL,              -- text | image | video (GIFs are image/gif uploads)
    text_content     TEXT,                       -- text stories only
    background_color TEXT,                       -- text stories only, "#RRGGBB"
    caption          TEXT,                       -- media stories only
    media_id         UUID REFERENCES media_files(id),
    privacy          TEXT NOT NULL DEFAULT 'contacts', -- contacts | contacts_except | selected
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at       TIMESTAMPTZ NOT NULL,
    deleted_at       TIMESTAMPTZ,
    CONSTRAINT chk_story_content CHECK (
        (story_type = 'text'  AND text_content IS NOT NULL AND media_id IS NULL)
     OR (story_type IN ('image', 'video') AND media_id IS NOT NULL)
    )
);

CREATE INDEX idx_stories_author_created ON stories (author_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_stories_media ON stories (media_id) WHERE media_id IS NOT NULL;

-- Per-story audience overrides. kind='exclude' for privacy=contacts_except,
-- kind='include' for privacy=selected. Unused for privacy=contacts.
CREATE TABLE story_audience (
    story_id UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    user_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind     TEXT NOT NULL,
    PRIMARY KEY (story_id, user_id)
);

-- Mentioned people can see the story regardless of the privacy setting
-- (unless blocked either way).
CREATE TABLE story_mentions (
    story_id UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    user_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (story_id, user_id)
);

CREATE TABLE story_views (
    story_id  UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    viewer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    viewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (story_id, viewer_id)
);
CREATE INDEX idx_story_views_viewer ON story_views (viewer_id);

CREATE TABLE story_reactions (
    story_id   UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (story_id, user_id)
);

CREATE TABLE story_replies (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    story_id   UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    replier_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_story_replies_story ON story_replies (story_id, created_at);

-- "Mute this person's stories": still listed (flagged muted) so they can be
-- unmuted, just sorted into a separate section client-side.
CREATE TABLE story_mutes (
    muter_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    muted_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (muter_id, muted_id)
);
