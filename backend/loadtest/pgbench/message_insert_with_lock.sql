-- Replicates internal/chat/repository.go's InsertMessage transaction
-- (idempotency pre-check, SELECT ... FOR UPDATE row lock on the
-- conversation, MAX(seq)+1, INSERT, conversation bump) against one
-- single busy group conversation shared by every virtual client — the
-- worst-case contention pattern (many devices posting into the same
-- group at once). All string/UUID values are resolved inside SQL
-- (not via pgbench's :'var' quoting, which this pgbench build does not
-- support) — only the numeric :client_id/:uidx substitution is used.
\set uidx (:client_id % 20)

BEGIN;
SELECT m.id FROM messages m
  WHERE m.conversation_id = '3bd68548-e53a-42cb-832e-d722406ff500'
    AND m.sender_id = (SELECT user_id FROM bench_user_pool WHERE idx = :uidx)
    AND m.client_tmp_id = gen_random_uuid()::text;
SELECT id FROM conversations WHERE id = '3bd68548-e53a-42cb-832e-d722406ff500' FOR UPDATE;
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
