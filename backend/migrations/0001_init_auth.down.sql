DROP TRIGGER IF EXISTS trg_user_profiles_updated_at ON user_profiles;
DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
DROP FUNCTION IF EXISTS set_updated_at();

DROP TABLE IF EXISTS otp_request_log;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS user_devices;
DROP TABLE IF EXISTS user_profiles;
DROP TABLE IF EXISTS users;
