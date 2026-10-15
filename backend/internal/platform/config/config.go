// Package config loads all runtime configuration from environment variables.
// No secret ever has a hardcoded default in source; missing required secrets
// fail startup loudly instead of falling back to something insecure.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env  string // "local" | "staging" | "production"
	HTTP struct {
		Port string
	}
	Postgres struct {
		DSN string
	}
	Redis struct {
		Addr       string
		Password   string
		DB         int
		TLSEnabled bool // true against ElastiCache with transit encryption on (terraform/elasticache.tf); false for local docker-compose Redis
	}
	JWT struct {
		AccessSecret  string
		RefreshSecret string
		AccessTTL     time.Duration
		RefreshTTL    time.Duration
	}
	OTP struct {
		Length              int
		TTL                 time.Duration
		MaxPerPhonePer10Min int
		MaxPerIPPer10Min    int
	}
	SMS struct {
		Provider string // pluggable: "console" (dev/log-only) | "twilio" | "custom"
		APIKey   string
	}
	S3 struct {
		Endpoint        string // internal endpoint the backend uses to talk to S3/MinIO
		PresignEndpoint string // public endpoint baked into presigned URLs handed to clients (see media/s3.go)
		Region          string
		Bucket          string
		AccessKeyID     string
		SecretAccessKey string
		UsePathStyle    bool          // required for MinIO; false for real AWS S3
		PresignTTL      time.Duration // how long presigned upload/download URLs stay valid
		PublicBaseURL   string        // CDN-ready: if set, download URLs are built as PublicBaseURL/bucket/key instead of presigned S3 URLs (set once a CDN sits in front of the bucket)
	}
	Media struct {
		MaxImageBytes    int64
		MaxVideoBytes    int64
		MaxAudioBytes    int64
		MaxDocumentBytes int64
	}
	Gemini struct {
		APIKey  string // required only to actually call the model; left empty the /v1/ai routes 500 on send, everything else still works
		Model   string
		BaseURL string
	}
	Admin struct {
		// Phone numbers (E.164, comma-separated) to promote to superadmin
		// on every startup — see internal/admin.Service.BootstrapSuperadmins.
		// This is how the very first superadmin ever gets created; every
		// later grant goes through the API and requires an existing one.
		BootstrapPhoneNumbers []string
	}
	Metrics struct {
		Port string // separate port for Prometheus /metrics — see cmd/api/main.go; not the public API port
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("config: required environment variable %s is not set", key))
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envInt64Or(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envBoolOr(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

// Load reads configuration from the process environment. In local/dev, the
// caller is expected to have already loaded a .env file (see cmd/api/main.go).
func Load() (*Config, error) {
	cfg := &Config{}
	cfg.Env = envOr("APP_ENV", "local")
	cfg.HTTP.Port = envOr("HTTP_PORT", "8080")

	cfg.Postgres.DSN = mustEnv("POSTGRES_DSN")

	cfg.Redis.Addr = mustEnv("REDIS_ADDR")
	cfg.Redis.Password = os.Getenv("REDIS_PASSWORD")
	cfg.Redis.DB = envIntOr("REDIS_DB", 0)
	cfg.Redis.TLSEnabled = envBoolOr("REDIS_TLS_ENABLED", false)

	cfg.JWT.AccessSecret = mustEnv("JWT_ACCESS_SECRET")
	cfg.JWT.RefreshSecret = mustEnv("JWT_REFRESH_SECRET")
	if cfg.JWT.AccessSecret == cfg.JWT.RefreshSecret {
		return nil, fmt.Errorf("config: JWT_ACCESS_SECRET and JWT_REFRESH_SECRET must differ")
	}
	cfg.JWT.AccessTTL = envDurationOr("JWT_ACCESS_TTL", 15*time.Minute)
	cfg.JWT.RefreshTTL = envDurationOr("JWT_REFRESH_TTL", 30*24*time.Hour)

	cfg.OTP.Length = envIntOr("OTP_LENGTH", 6)
	cfg.OTP.TTL = envDurationOr("OTP_TTL", 5*time.Minute)
	cfg.OTP.MaxPerPhonePer10Min = envIntOr("OTP_MAX_PER_PHONE_10M", 3)
	cfg.OTP.MaxPerIPPer10Min = envIntOr("OTP_MAX_PER_IP_10M", 10)

	cfg.SMS.Provider = envOr("SMS_PROVIDER", "console")
	cfg.SMS.APIKey = os.Getenv("SMS_API_KEY")

	cfg.S3.Endpoint = envOr("S3_ENDPOINT", "")
	cfg.S3.PresignEndpoint = envOr("S3_PRESIGN_ENDPOINT", "")
	cfg.S3.Region = envOr("S3_REGION", "ap-southeast-1")
	cfg.S3.Bucket = mustEnv("S3_BUCKET")
	cfg.S3.AccessKeyID = mustEnv("S3_ACCESS_KEY_ID")
	cfg.S3.SecretAccessKey = mustEnv("S3_SECRET_ACCESS_KEY")
	cfg.S3.UsePathStyle = envBoolOr("S3_USE_PATH_STYLE", cfg.S3.Endpoint != "")
	cfg.S3.PresignTTL = envDurationOr("S3_PRESIGN_TTL", 15*time.Minute)
	cfg.S3.PublicBaseURL = os.Getenv("S3_PUBLIC_BASE_URL")

	cfg.Media.MaxImageBytes = envInt64Or("MEDIA_MAX_IMAGE_BYTES", 15*1024*1024)        // 15MB
	cfg.Media.MaxVideoBytes = envInt64Or("MEDIA_MAX_VIDEO_BYTES", 200*1024*1024)       // 200MB
	cfg.Media.MaxAudioBytes = envInt64Or("MEDIA_MAX_AUDIO_BYTES", 25*1024*1024)        // 25MB (covers voice notes)
	cfg.Media.MaxDocumentBytes = envInt64Or("MEDIA_MAX_DOCUMENT_BYTES", 100*1024*1024) // 100MB

	// Not mustEnv: an empty key is a valid "AI module not configured yet"
	// state in local/dev — every other /v1/ai route still works, only the
	// actual Gemini call fails, loudly, at request time. Never log this value.
	cfg.Gemini.APIKey = os.Getenv("GEMINI_API_KEY")
	cfg.Gemini.Model = envOr("GEMINI_MODEL", "gemini-2.0-flash")
	cfg.Gemini.BaseURL = envOr("GEMINI_BASE_URL", "")

	if raw := os.Getenv("ADMIN_BOOTSTRAP_PHONE_NUMBERS"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				cfg.Admin.BootstrapPhoneNumbers = append(cfg.Admin.BootstrapPhoneNumbers, p)
			}
		}
	}

	cfg.Metrics.Port = envOr("METRICS_PORT", "9090")

	return cfg, nil
}
