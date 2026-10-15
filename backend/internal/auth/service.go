package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/yourorg/chatplatform/internal/platform/audit"
)

var (
	ErrInvalidPhoneNumber = errors.New("auth: invalid phone number")
	ErrRefreshTokenReused = errors.New("auth: refresh token reuse detected")
	ErrSessionExpired     = errors.New("auth: session expired or revoked")
	ErrAccountSuspended   = errors.New("auth: account is suspended")
)

type Service struct {
	repo  *Repository
	otp   *OTPService
	jwt   *JWTService
	audit *audit.Logger // nil-safe: audit.Logger.Log no-ops on a nil receiver
	email EmailAuth     // e-mail/password flow collaborators (see email_service.go)
}

func NewService(repo *Repository, otp *OTPService, jwt *JWTService, auditLogger *audit.Logger) *Service {
	return &Service{repo: repo, otp: otp, jwt: jwt, audit: auditLogger}
}

func (s *Service) RequestOTP(ctx context.Context, phoneNumber, ipAddress string) error {
	if !isValidE164(phoneNumber) {
		return ErrInvalidPhoneNumber
	}
	err := s.otp.RequestOTP(ctx, phoneNumber, ipAddress)
	if err == nil {
		// No user_id yet at this point (phone may not even be registered) —
		// phoneNumber itself is deliberately NOT stored in metadata (PII
		// minimization); the ip_address column is what ties repeated
		// requests together for abuse review.
		s.audit.Log(ctx, audit.EventOTPRequested, "", ipAddress, "", nil)
	}
	return err
}

// VerifyOTP completes login: validates the code, finds-or-creates the user,
// registers the device, and issues a fresh token pair. This is the only
// place a new session is minted from a phone+code (not from a refresh token).
func (s *Service) VerifyOTP(ctx context.Context, req VerifyOTPRequest, ip, userAgent string) (AuthResponse, error) {
	if !isValidE164(req.PhoneNumber) {
		return AuthResponse{}, ErrInvalidPhoneNumber
	}
	if err := s.otp.VerifyOTP(ctx, req.PhoneNumber, req.Code); err != nil {
		s.audit.Log(ctx, audit.EventOTPVerifyFailed, "", ip, userAgent, nil)
		return AuthResponse{}, err
	}

	countryCode := countryCodeFromPhone(req.PhoneNumber)
	user, isNew, err := s.repo.FindOrCreateUserByPhone(ctx, req.PhoneNumber, countryCode)
	if err != nil {
		return AuthResponse{}, fmt.Errorf("verify otp: find or create user: %w", err)
	}
	// Phase 11: a correct OTP alone is no longer enough — a suspended
	// account (internal/admin) is blocked here, at the one place new
	// sessions are minted. No tokens are issued, so there's nothing to
	// revoke; an already-suspended user's *existing* sessions are revoked
	// at suspension time instead (see auth.Service.SuspendUser).
	if user.Status == "suspended" {
		s.audit.Log(ctx, audit.EventLoginBlockedSuspend, user.ID, ip, userAgent, nil)
		return AuthResponse{}, ErrAccountSuspended
	}

	device, err := s.repo.UpsertDevice(ctx, user.ID, req.DeviceID, req.Platform, req.PushToken, req.AppVersion)
	if err != nil {
		return AuthResponse{}, fmt.Errorf("verify otp: upsert device: %w", err)
	}

	tokens, err := s.mintTokenPair(ctx, user.ID, device.ID, req.DeviceID, ip, userAgent, nil)
	if err != nil {
		return AuthResponse{}, err
	}
	s.audit.Log(ctx, audit.EventLoginSucceeded, user.ID, ip, userAgent, map[string]any{"is_new_user": isNew})

	return AuthResponse{
		User:   PublicUser{ID: user.ID, PhoneNumber: user.PhoneNumber, Username: user.Username},
		Tokens: tokens,
		IsNew:  isNew,
	}, nil
}

// mintTokenPair issues an access JWT + a freshly generated refresh token and
// persists the refresh token's hash as a new session row.
func (s *Service) mintTokenPair(ctx context.Context, userID, deviceRowID, deviceClientID, ip, userAgent string, parentSessionID *string) (TokenPair, error) {
	accessToken, accessExp, err := s.jwt.IssueAccessToken(userID, deviceClientID)
	if err != nil {
		return TokenPair{}, err
	}

	rawRefresh, refreshHash, err := GenerateRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}
	refreshExp := time.Now().Add(s.jwt.RefreshTTL())

	if _, err := s.repo.CreateSession(ctx, userID, deviceRowID, refreshHash, ip, userAgent, refreshExp, parentSessionID); err != nil {
		return TokenPair{}, fmt.Errorf("mint tokens: create session: %w", err)
	}

	return TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     rawRefresh,
		AccessExpiresAt:  accessExp,
		RefreshExpiresAt: refreshExp,
	}, nil
}

// Refresh rotates a refresh token: the presented token is looked up by hash,
// checked for validity, immediately revoked, and replaced by a new one
// chained via parent_session_id. If a token that was already revoked (i.e.
// already rotated away, or explicitly logged out) is presented again, that
// is treated as reuse of a stolen/leaked token and the entire session family
// for that user is revoked defensively.
func (s *Service) Refresh(ctx context.Context, rawRefreshToken, ip, userAgent string) (TokenPair, error) {
	hash := HashRefreshToken(rawRefreshToken)
	session, err := s.repo.GetSessionByRefreshHash(ctx, hash)
	if errors.Is(err, ErrNotFound) {
		return TokenPair{}, ErrSessionExpired
	}
	if err != nil {
		return TokenPair{}, fmt.Errorf("refresh: lookup session: %w", err)
	}

	if session.RevokedAt != nil {
		_ = s.repo.RevokeAllSessionsForUser(ctx, session.UserID, "reuse_detected")
		s.audit.Log(ctx, audit.EventRefreshTokenReused, session.UserID, ip, userAgent, nil)
		return TokenPair{}, ErrRefreshTokenReused
	}
	if time.Now().After(session.ExpiresAt) {
		return TokenPair{}, ErrSessionExpired
	}

	if err := s.repo.RevokeSession(ctx, session.ID, "rotated"); err != nil {
		return TokenPair{}, fmt.Errorf("refresh: revoke old session: %w", err)
	}

	parentID := session.ID
	return s.mintTokenPair(ctx, session.UserID, session.DeviceRowID, "", ip, userAgent, &parentID)
}

func (s *Service) Logout(ctx context.Context, rawRefreshToken, ip, userAgent string) error {
	hash := HashRefreshToken(rawRefreshToken)
	session, err := s.repo.GetSessionByRefreshHash(ctx, hash)
	if errors.Is(err, ErrNotFound) {
		return nil // already gone; logout is idempotent
	}
	if err != nil {
		return err
	}
	if err := s.repo.RevokeSession(ctx, session.ID, "logout"); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.EventLogoutSucceeded, session.UserID, ip, userAgent, nil)
	return nil
}

func (s *Service) ListSessions(ctx context.Context, userID string) ([]SessionInfo, error) {
	return s.repo.ListActiveSessions(ctx, userID)
}

// RevokeSession revokes sessionID only if it belongs to userID (the
// authenticated caller from the JWT) — never trusts the session ID alone.
func (s *Service) RevokeSession(ctx context.Context, sessionID, userID, ip, userAgent string) error {
	if err := s.repo.RevokeSessionForUser(ctx, sessionID, userID, "user_revoked"); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.EventSessionRevoked, userID, ip, userAgent, map[string]any{"session_id": sessionID})
	return nil
}

// RevokeAllSessions is the "I think my account is compromised" / "log out
// everywhere" action — it revokes every active session for userID,
// including whichever one the caller is using right now (the access token
// already issued stays valid until it naturally expires — access tokens
// are short-lived and not looked up per-request, by design; only the
// refresh token is revocable). The client should treat a successful call
// as "the user is about to be logged out everywhere" and discard its own
// tokens immediately rather than waiting to be rejected.
func (s *Service) RevokeAllSessions(ctx context.Context, userID, ip, userAgent string) error {
	if err := s.repo.RevokeAllSessionsForUser(ctx, userID, "user_revoked_all"); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.EventAllSessionsRevoked, userID, ip, userAgent, nil)
	return nil
}

func (s *Service) VerifyAccessToken(token string) (string, string, error) {
	return s.jwt.VerifyAccessToken(token)
}

// DebugPeekOTP exposes the stored OTP for local testing only — see
// handler.go MountDebugRoutes for the enforcement of that restriction.
func (s *Service) DebugPeekOTP(ctx context.Context, phoneNumber string) (string, error) {
	return s.otp.PeekCode(ctx, phoneNumber)
}

// --- Phase 11: the surface internal/admin consumes. Every method here
// returns only primitive types / maps, never a *Service-specific struct,
// so admin's interfaces can be satisfied structurally without admin
// importing this package (see admin_repository.go's doc comment, and
// internal/communities etc. for the same pattern used throughout this
// project). None of this performs its own authorization — the caller
// (internal/admin) is responsible for checking the acting user actually
// holds an admin role before calling any of it.

func (s *Service) GetUserIDByPhone(ctx context.Context, phoneNumber string) (string, error) {
	return s.repo.GetUserIDByPhone(ctx, phoneNumber)
}

func userToMap(u User) map[string]any {
	return map[string]any{
		"id": u.ID, "phone_number": u.PhoneNumber, "country_code": u.CountryCode,
		"username": u.Username, "status": u.Status, "created_at": u.CreatedAt,
	}
}

func (s *Service) GetUserAdmin(ctx context.Context, userID string) (map[string]any, error) {
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return userToMap(u), nil
}

func (s *Service) SearchUsersAdmin(ctx context.Context, query string, limit, offset int) ([]map[string]any, int, error) {
	users, total, err := s.repo.SearchUsersAdmin(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]map[string]any, len(users))
	for i, u := range users {
		out[i] = userToMap(u)
	}
	return out, total, nil
}

// SuspendUser blocks future logins immediately (VerifyOTP checks status)
// and revokes every currently-active session so the user is logged out
// everywhere within the lifetime of their current access token (same
// access-token caveat as RevokeAllSessions — stateless tokens already
// issued remain valid until they naturally expire).
func (s *Service) SuspendUser(ctx context.Context, targetUserID, reason, actingAdminID string) error {
	if err := s.repo.SetUserStatus(ctx, targetUserID, "suspended", &reason, &actingAdminID); err != nil {
		return err
	}
	_ = s.repo.RevokeAllSessionsForUser(ctx, targetUserID, "suspended_by_admin")
	s.audit.Log(ctx, audit.EventUserSuspended, actingAdminID, "", "", map[string]any{
		"target_user_id": targetUserID, "reason": reason,
	})
	return nil
}

func (s *Service) UnsuspendUser(ctx context.Context, targetUserID, actingAdminID string) error {
	if err := s.repo.SetUserStatus(ctx, targetUserID, "active", nil, nil); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.EventUserUnsuspended, actingAdminID, "", "", map[string]any{
		"target_user_id": targetUserID,
	})
	return nil
}
