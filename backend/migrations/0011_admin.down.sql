ALTER TABLE channel_reports
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS resolved_by,
    DROP COLUMN IF EXISTS resolution_note;

ALTER TABLE users
    DROP COLUMN IF EXISTS suspended_reason,
    DROP COLUMN IF EXISTS suspended_at,
    DROP COLUMN IF EXISTS suspended_by;

DROP TABLE IF EXISTS admin_roles;
