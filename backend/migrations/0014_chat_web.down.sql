ALTER TABLE conversation_members
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS pinned_at;
