DROP TRIGGER IF EXISTS trg_groups_updated_at ON groups;

DROP INDEX IF EXISTS idx_messages_search_vector;
ALTER TABLE messages DROP COLUMN IF EXISTS search_vector;
ALTER TABLE messages DROP COLUMN IF EXISTS forwarded_from_message_id;

DROP TABLE IF EXISTS message_reactions;
DROP TABLE IF EXISTS groups;

ALTER TABLE conversation_members DROP COLUMN IF EXISTS role;
