-- 0016: change e-mail without locking anyone out. The new address waits in pending_email until its owner clicks the
-- e-mailed link (email_verification_tokens.email = the new address); only then does users.email switch.
ALTER TABLE users ADD COLUMN pending_email TEXT;
ALTER TABLE users ADD CONSTRAINT chk_users_pending_email_lower CHECK (pending_email IS NULL OR pending_email = lower(pending_email));
