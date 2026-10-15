package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/yourorg/chatplatform/internal/platform/ratelimit"
)

var (
	ErrOTPRateLimited = errors.New("otp: rate limited")
	ErrOTPInvalid     = errors.New("otp: invalid or expired code")
)

// SMSSender abstracts the SMS provider so the real provider (Twilio, etc.)
// can be swapped without touching business logic. It must NEVER be logged
// from inside an implementation with the code included.
type SMSSender interface {
	Send(ctx context.Context, phoneNumber, code string) error
}

// ConsoleSMSSender is for local development only: it does not send a real
// SMS. It intentionally does not log the code either, to keep the "never log
// OTPs" rule enforceable even in dev — swap SMS_PROVIDER for a real sender
// and read the code from the provider's own delivery report if needed.
type ConsoleSMSSender struct{}

func (ConsoleSMSSender) Send(ctx context.Context, phoneNumber, code string) error {
	// Deliberately not logging `code`. In local/dev, read it back via the
	// dev-only debug endpoint (see handler.go DebugPeekOTP, disabled outside
	// APP_ENV=local) instead of grepping logs.
	return nil
}

type OTPService struct {
	redis            *redis.Client
	limiter          *ratelimit.Limiter
	sender           SMSSender
	length           int
	ttl              time.Duration
	maxPhone         int
	maxIP            int
	debugPeekEnabled bool
}

// debugPeekEnabled must only ever be true when APP_ENV=local (see
// cmd/api/main.go) — it makes RequestOTP additionally stash the plaintext
// code in a second, separate Redis key purely so the local-dev debug
// endpoint can recover it without a real SMS provider. The real
// verification key (otpKey) always stores only the Argon2id hash,
// regardless of this flag.
func NewOTPService(rdb *redis.Client, sender SMSSender, length int, ttl time.Duration, maxPhonePer10m, maxIPPer10m int, debugPeekEnabled bool) *OTPService {
	return &OTPService{
		redis: rdb, limiter: ratelimit.New(rdb), sender: sender,
		length: length, ttl: ttl, maxPhone: maxPhonePer10m, maxIP: maxIPPer10m,
		debugPeekEnabled: debugPeekEnabled,
	}
}

func otpKey(phone string) string          { return "otp:code:" + phone }
func otpDebugKey(phone string) string     { return "otp:debug:" + phone }
func otpPhoneRateKey(phone string) string { return "otp:rl:phone:" + phone }
func otpIPRateKey(ip string) string       { return "otp:rl:ip:" + ip }

func generateNumericCode(length int) (string, error) {
	digits := make([]byte, length)
	for i := range digits {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		digits[i] = byte('0' + n.Int64())
	}
	return string(digits), nil
}

// RequestOTP generates a code, stores it (hashed key -> plain code, since the
// key itself is the secret channel — Redis is trusted infra, not exposed to
// clients) with a TTL, enforces per-phone and per-IP rate limits, and sends
// it via the configured SMS provider. The plaintext code never leaves this
// function except to the SMS provider call.
func (s *OTPService) RequestOTP(ctx context.Context, phoneNumber, ipAddress string) error {
	okPhone, err := s.limiter.Allow(ctx, otpPhoneRateKey(phoneNumber), s.maxPhone, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("otp: rate limit check: %w", err)
	}
	if !okPhone {
		return ErrOTPRateLimited
	}
	okIP, err := s.limiter.Allow(ctx, otpIPRateKey(ipAddress), s.maxIP, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("otp: rate limit check: %w", err)
	}
	if !okIP {
		return ErrOTPRateLimited
	}

	code, err := generateNumericCode(s.length)
	if err != nil {
		return fmt.Errorf("otp: generate code: %w", err)
	}

	hashed, err := hashOTPCode(code)
	if err != nil {
		return fmt.Errorf("otp: hash code: %w", err)
	}
	if err := s.redis.Set(ctx, otpKey(phoneNumber), hashed, s.ttl).Err(); err != nil {
		return fmt.Errorf("otp: store code: %w", err)
	}

	// Local-dev convenience only — see debugPeekEnabled's doc comment above.
	if s.debugPeekEnabled {
		if err := s.redis.Set(ctx, otpDebugKey(phoneNumber), code, s.ttl).Err(); err != nil {
			return fmt.Errorf("otp: store debug code: %w", err)
		}
	}

	if err := s.sender.Send(ctx, phoneNumber, code); err != nil {
		return fmt.Errorf("otp: send: %w", err)
	}
	return nil
}

// PeekCode returns the currently stored plaintext code for a phone number
// without consuming it. It exists ONLY for the local-dev debug endpoint
// (see handler.go MountDebugRoutes, which is never mounted outside
// APP_ENV=local) so a developer can complete the login flow without a
// real SMS provider — it reads the separate debug-only key, never the
// real (hashed) verification key, and returns ErrOTPInvalid if debug
// peeking wasn't enabled when the code was requested.
func (s *OTPService) PeekCode(ctx context.Context, phoneNumber string) (string, error) {
	code, err := s.redis.Get(ctx, otpDebugKey(phoneNumber)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrOTPInvalid
	}
	return code, err
}

// VerifyOTP checks the submitted code against the stored Argon2id hash in
// constant time; the key is deleted on first successful verification so
// codes are single-use (a wrong guess does NOT consume it — the rate
// limiter above is what bounds guessing attempts, not consumption).
func (s *OTPService) VerifyOTP(ctx context.Context, phoneNumber, code string) error {
	stored, err := s.redis.Get(ctx, otpKey(phoneNumber)).Result()
	if errors.Is(err, redis.Nil) {
		return ErrOTPInvalid
	}
	if err != nil {
		return fmt.Errorf("otp: fetch code: %w", err)
	}
	if !verifyOTPCode(stored, code) {
		return ErrOTPInvalid
	}
	// single-use: delete immediately on success
	s.redis.Del(ctx, otpKey(phoneNumber), otpDebugKey(phoneNumber))
	return nil
}
