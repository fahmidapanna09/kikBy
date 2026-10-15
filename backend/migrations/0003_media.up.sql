-- 0003_media.up.sql
-- Phase 3: object-storage-backed media (images, video, audio/voice messages,
-- documents), linked to messages via message_attachments.

CREATE TABLE media_files (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bucket              TEXT NOT NULL,
    object_key          TEXT NOT NULL,
    kind                TEXT NOT NULL,          -- image | video | audio | voice | document
    declared_mime_type  TEXT NOT NULL,          -- what the client said it would upload
    confirmed_mime_type TEXT,                   -- what S3 actually reports after upload (HEAD), NULL until confirmed
    size_bytes          BIGINT,                 -- filled in on confirm, from S3 HEAD, never trusted from the client alone
    width               INT,                    -- images/video only
    height              INT,
    duration_ms         INT,                    -- audio/video/voice only
    thumbnail_object_key TEXT,                  -- populated by the thumbnail worker, NULL until generated
    status              TEXT NOT NULL DEFAULT 'pending_upload', -- pending_upload | uploaded | rejected | deleted
    rejection_reason    TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at        TIMESTAMPTZ,
    deleted_at          TIMESTAMPTZ
);

CREATE INDEX idx_media_files_owner ON media_files (owner_id);
CREATE INDEX idx_media_files_status_pending ON media_files (created_at) WHERE status = 'pending_upload';

-- A message can carry more than one attachment (e.g. multiple images sent
-- together), so this is a join table rather than a single nullable FK on
-- messages. position lets the client render them in the order chosen.
CREATE TABLE message_attachments (
    message_id  UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    media_id    UUID NOT NULL REFERENCES media_files(id) ON DELETE RESTRICT,
    position    INT NOT NULL DEFAULT 0,
    PRIMARY KEY (message_id, media_id)
);

CREATE INDEX idx_message_attachments_media ON message_attachments (media_id);
