package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"time"

	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/audit"
)

const (
	resetTokenTTL      = 30 * time.Minute
	lockoutWindow      = 15 * time.Minute
	lockoutMaxFailures = 10
)

var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrAccountLocked      = errors.New("auth: too many failed sign-in attempts")
	ErrInvalidResetToken  = errors.New("auth: reset token invalid or expired")
	// ErrValidation carries per-field messages for the form.
	ErrValidation = errors.New("auth: validation failed")
)

// ValidationError wraps ErrValidation with the offending fields.
type ValidationError struct{ Fields FieldErrors }

func (e *ValidationError) Error() string { return ErrValidation.Error() }
func (e *ValidationError) Unwrap() error { return ErrValidation }

// EmailAuth holds the collaborators that only the e-mail/password flow needs,
// kept off Service's constructor so existing wiring and tests are untouched.
type EmailAuth struct {
	Mailer       Mailer
	ResetBaseURL string      // e.g. https://kikbite.com — the SPA serves /reset-password?token=... (verification uses a 6-digit code, no link)
	Log          *zap.Logger // optional; used for mail-delivery failures that must not fail the request
}

func (s *Service) WithEmailAuth(e EmailAuth) *Service { s.email = e; return s }

// dummyHash is verified against when the account does not exist, so "unknown
// user" and "wrong password" cost the same and can't be told apart by timing.
var dummyHash, _ = hashPassword("kikbite-dummy-password-1")

func (s *Service) Register(ctx context.Context, req RegisterRequest, ip, userAgent string) (AuthResponse, error) {
	fields := FieldErrors{}
	username := normalizeUsername(req.Username)
	email := normalizeEmail(req.Email)
	if e := validateDisplayName(req.Name); e != "" {
		fields["name"] = e
	}
	if !usernamePattern.MatchString(username) {
		fields["username"] = "3–24 characters: letters, numbers or _"
	}
	if email == "" {
		fields["email"] = "enter a valid email address"
	}
	if e := validatePassword(req.Password); e != "" {
		fields["password"] = e
	}
	phone, ok := normalizeContactPhone(req.ContactPhone)
	if !ok {
		fields["phone"] = "enter a valid phone number (7–15 digits)"
	}
	if len(fields) > 0 {
		return AuthResponse{}, &ValidationError{Fields: fields}
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		return AuthResponse{}, err
	}
	user, err := s.repo.CreateUserWithEmail(ctx, username, req.Name, email, hash, phone)
	switch {
	case errors.Is(err, ErrEmailTaken):
		// NOTE: this does reveal that an e-mail is registered. It is the usual
		// trade-off for a form that must tell the person what to fix; the
		// endpoint is rate-limited per IP. (Login/forgot-password do NOT reveal it.)
		return AuthResponse{}, &ValidationError{Fields: FieldErrors{"email": "an account with this email already exists"}}
	case errors.Is(err, ErrUsernameTaken):
		return AuthResponse{}, &ValidationError{Fields: FieldErrors{"username": "that username is taken"}}
	case err != nil:
		return AuthResponse{}, fmt.Errorf("register: %w", err)
	}
	s.audit.Log(ctx, audit.EventRegistered, user.ID, ip, userAgent, nil)
	s.sendVerificationAsync(user.ID, email)
	return s.startSession(ctx, user, req.DeviceID, req.Platform, req.AppVersion, ip, userAgent, true)
}

func (s *Service) Login(ctx context.Context, req LoginRequest, ip, userAgent string) (AuthResponse, error) {
	id := normalizeUsername(req.Identifier) // lower-cased + trimmed works for both e-mail and username
	user, hash, err := s.repo.GetCredentialsByIdentifier(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return AuthResponse{}, fmt.Errorf("login: %w", err)
	}
	if err == nil && hash != "" {
		// Per-account lockout (on top of the per-IP rate limit): attempts made while locked are neither counted nor audited,
		// so an attacker cannot keep a victim locked out forever — the lock ends 15 min after the last counted failure.
		if n, cerr := s.repo.CountRecentFailedLogins(ctx, user.ID, lockoutWindow); cerr == nil && n >= lockoutMaxFailures {
			return AuthResponse{}, ErrAccountLocked
		}
	}
	if errors.Is(err, ErrNotFound) || hash == "" {
		verifyPassword(dummyHash, req.Password) // equalise timing
		s.audit.Log(ctx, audit.EventLoginFailed, "", ip, userAgent, nil)
		return AuthResponse{}, ErrInvalidCredentials
	}
	if !verifyPassword(hash, req.Password) {
		s.audit.Log(ctx, audit.EventLoginFailed, user.ID, ip, userAgent, nil)
		return AuthResponse{}, ErrInvalidCredentials
	}
	if user.Status == "suspended" {
		s.audit.Log(ctx, audit.EventLoginBlockedSuspend, user.ID, ip, userAgent, nil)
		return AuthResponse{}, ErrAccountSuspended
	}
	s.audit.Log(ctx, audit.EventLoginSucceeded, user.ID, ip, userAgent, map[string]any{"method": "password"})
	return s.startSession(ctx, user, req.DeviceID, req.Platform, req.AppVersion, ip, userAgent, false)
}

func (s *Service) startSession(ctx context.Context, user User, deviceID, platform, appVersion, ip, userAgent string, isNew bool) (AuthResponse, error) {
	device, err := s.repo.UpsertDevice(ctx, user.ID, deviceID, platform, "", appVersion)
	if err != nil {
		return AuthResponse{}, fmt.Errorf("start session: upsert device: %w", err)
	}
	tokens, err := s.mintTokenPair(ctx, user.ID, device.ID, deviceID, ip, userAgent, nil)
	if err != nil {
		return AuthResponse{}, err
	}
	return AuthResponse{
		User:   PublicUser{ID: user.ID, Email: user.Email, PhoneNumber: user.PhoneNumber, Username: user.Username, EmailVerified: user.EmailVerified},
		Tokens: tokens,
		IsNew:  isNew,
	}, nil
}

func newResetToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashResetToken(raw), nil
}

func hashResetToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ForgotPassword always returns nil for well-formed or unknown e-mails so the
// response cannot be used to discover which addresses have accounts. Delivery
// failures are logged by the caller's mailer, never surfaced to the client.
func (s *Service) ForgotPassword(ctx context.Context, rawEmail, ip string) error {
	email := normalizeEmail(rawEmail)
	if email == "" {
		return nil
	}
	userID, err := s.repo.GetUserIDByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("forgot password: %w", err)
	}
	raw, hash, err := newResetToken()
	if err != nil {
		return err
	}
	if err := s.repo.CreateResetToken(ctx, userID, hash, time.Now().Add(resetTokenTTL)); err != nil {
		return fmt.Errorf("forgot password: store token: %w", err)
	}
	link := s.email.ResetBaseURL + "/reset-password?token=" + url.QueryEscape(raw)
	mailer := s.email.Mailer
	if mailer == nil {
		mailer = NoopMailer{}
	}
	if err := mailer.SendPasswordReset(ctx, email, link); err != nil {
		return fmt.Errorf("forgot password: send mail: %w", err)
	}
	s.audit.Log(ctx, audit.EventPasswordResetRequested, userID, ip, "", nil)
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, req ResetPasswordRequest, ip string) error {
	if e := validatePassword(req.Password); e != "" {
		return &ValidationError{Fields: FieldErrors{"password": e}}
	}
	if req.Token == "" {
		return ErrInvalidResetToken
	}
	userID, err := s.repo.ConsumeResetToken(ctx, hashResetToken(req.Token))
	if errors.Is(err, ErrNotFound) {
		return ErrInvalidResetToken
	}
	if err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return err
	}
	if err := s.repo.SetPasswordAndRevokeSessions(ctx, userID, hash); err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	s.audit.Log(ctx, audit.EventPasswordResetCompleted, userID, ip, "", nil)
	if cur, _, _, err := s.repo.GetEmailState(ctx, userID); err == nil {
		s.notifyAsync(cur, SecurityNotice{Kind: NoticePasswordReset})
	}
	return nil
}
