-- 0001_init_auth.up.sql
-- Phase 1: users, user_profiles, user_devices, user_sessions

CREATE EXTENSION IF NOT EXISTS pgcrypto; -- gen_random_uuid()

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_number    TEXT NOT NULL,       -- E.164, e.g. +8801XXXXXXXXX
    country_code    TEXT NOT NULL,       -- e.g. "BD", "US"
    username        TEXT,                -- optional, unique when set
    status          TEXT NOT NULL DEFAULT 'active', -- active | suspended | deleted
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_users_phone_number ON users (phone_number) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_users_username ON users (lower(username)) WHERE deleted_at IS NULL AND username IS NOT NULL;

CREATE TABLE user_profiles (
    user_id         UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    display_name    TEXT,
    about           TEXT,
    avatar_media_id UUID,          -- FK to media_files, added in a later migration
    last_seen_at    TIMESTAMPTZ,
    last_seen_visibility TEXT NOT NULL DEFAULT 'everyone', -- everyone|contacts|nobody
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_devices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       TEXT NOT NULL,       -- client-generated stable device identifier
    platform        TEXT NOT NULL,       -- android | ios | web
    push_token      TEXT,                -- FCM token, nullable
    app_version     TEXT,
    last_active_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_user_devices_user_device ON user_devices (user_id, device_id);
CREATE INDEX idx_user_devices_user_id ON user_devices (user_id) WHERE revoked_at IS NULL;

-- One row per issued refresh token. Access tokens are stateless JWTs and are
-- NOT stored here; only the refresh token (hashed) and session metadata are.
CREATE TABLE user_sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id           UUID NOT NULL REFERENCES user_devices(id) ON DELETE CASCADE,
    refresh_token_hash  TEXT NOT NULL,      -- SHA-256 hex of the refresh token
    parent_session_id   UUID REFERENCES user_sessions(id), -- rotation chain, for reuse detection
    ip_address          INET,
    user_agent          TEXT,
    issued_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    revoked_reason      TEXT                -- logout | rotated | reuse_detected | admin | expired
);

CREATE UNIQUE INDEX uq_user_sessions_refresh_hash ON user_sessions (refresh_token_hash);
CREATE INDEX idx_user_sessions_user_active ON user_sessions (user_id) WHERE revoked_at IS NULL;

-- OTP attempts are logged (not the code itself) purely for abuse detection / rate limiting audits.
-- The live OTP value+expiry lives in Redis, never in Postgres, and is never logged anywhere.
CREATE TABLE otp_request_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_number    TEXT NOT NULL,
    ip_address      INET,
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    purpose         TEXT NOT NULL DEFAULT 'login' -- login | phone_change | account_recovery
);

CREATE INDEX idx_otp_request_log_phone_time ON otp_request_log (phone_number, requested_at);
CREATE INDEX idx_otp_request_log_ip_time ON otp_request_log (ip_address, requested_at);

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_user_profiles_updated_at BEFORE UPDATE ON user_profiles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
