DROP TRIGGER IF EXISTS trg_conversations_updated_at ON conversations;

DROP TABLE IF EXISTS notification_outbox;
DROP TABLE IF EXISTS push_tokens;
DROP TABLE IF EXISTS message_reads;
DROP TABLE IF EXISTS message_deliveries;
DROP TABLE IF EXISTS message_deletions;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversation_members;
DROP TABLE IF EXISTS conversations;
