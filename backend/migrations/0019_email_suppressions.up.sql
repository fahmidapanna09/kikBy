-- 0019: addresses the mail provider reported as permanently undeliverable or that marked us as spam.
-- Sending to them is skipped (repeat bounces hurt the sending domain's reputation).
CREATE TABLE IF NOT EXISTS email_suppressions (
    email      TEXT PRIMARY KEY,           -- lower-cased
    reason     TEXT NOT NULL,              -- bounced | complained
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
