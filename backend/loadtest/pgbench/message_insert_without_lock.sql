-- Same transaction as message_insert_with_lock.sql, with the
-- "SELECT ... FOR UPDATE" row lock deliberately removed, to show what
-- the real code's comment ("so concurrent sends from different devices
-- never collide on seq") is actually protecting against. The
-- uq_messages_conversation_seq UNIQUE constraint (migration 0002) is
-- the backstop this comparison is really testing.
\set uidx (:client_id % 20)

BEGIN;
SELECT m.id FROM messages m
  WHERE m.conversation_id = '3bd68548-e53a-42cb-832e-d722406ff500'
    AND m.sender_id = (SELECT user_id FROM bench_user_pool WHERE idx = :uidx)
    AND m.client_tmp_id = gen_random_uuid()::text;
WITH ins AS (
  INSERT INTO messages (conversation_id, seq, sender_id, client_tmp_id, type, body)
  SELECT '3bd68548-e53a-42cb-832e-d722406ff500',
         (SELECT COALESCE(MAX(seq), 0) + 1 FROM messages WHERE conversation_id = '3bd68548-e53a-42cb-832e-d722406ff500'),
         (SELECT user_id FROM bench_user_pool WHERE idx = :uidx),
         gen_random_uuid()::text, 'text', 'bench message'
  RETURNING id
)
UPDATE conversations SET last_message_id = ins.id, last_message_at = now()
  FROM ins WHERE conversations.id = '3bd68548-e53a-42cb-832e-d722406ff500';
COMMIT;
