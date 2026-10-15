-- 0017: keep the user's original file name so chat can show "report.pdf"
-- instead of an opaque object key.
ALTER TABLE media_files ADD COLUMN IF NOT EXISTS original_name TEXT;
