package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/audit"
	"github.com/yourorg/chatplatform/internal/platform/middleware"
	"github.com/yourorg/chatplatform/internal/platform/response"
)

/*
Account management for signed-in e-mail/password users: change password, change e-mail, delete account.

Wrong-password answers are 422 "validation" with a field, NOT 401: the web client treats a 401 as "session expired"
(refresh, then sign out), and a typo in a confirmation field must never log anybody out.
*/

type ChangePasswordRequest struct {
	Current string `json:"current_password"`
	Next    string `json:"new_password"`
}
type ChangeEmailRequest struct {
	Password string `json:"password"`
	NewEmail string `json:"new_email"`
}
type DeleteAccountRequest struct {
	Password string `json:"password"`
}

// verifyOwnPassword checks the signed-in user's current password. Accounts without a password (legacy phone/OTP-only)
// cannot confirm anything this way, so they are refused with the same field error.
func (s *Service) verifyOwnPassword(ctx context.Context, userID, password, field string) error {
	hash, err := s.repo.GetPasswordHash(ctx, userID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if hash == "" {
		verifyPassword(dummyHash, password) // keep timing uniform
		return &ValidationError{Fields: FieldErrors{field: "incorrect password"}}
	}
	if !verifyPassword(hash, password) {
		return &ValidationError{Fields: FieldErrors{field: "incorrect password"}}
	}
	return nil
}

// ChangePassword sets a new password and signs the account out of every OTHER device (this one stays signed in).
func (s *Service) ChangePassword(ctx context.Context, userID, deviceClientID string, req ChangePasswordRequest, ip string) error {
	if e := validatePassword(req.Next); e != "" {
		return &ValidationError{Fields: FieldErrors{"new_password": e}}
	}
	if req.Next == req.Current {
		return &ValidationError{Fields: FieldErrors{"new_password": "choose a password you haven't used just now"}}
	}
	if err := s.verifyOwnPassword(ctx, userID, req.Current, "current"); err != nil {
		return err
	}
	hash, err := hashPassword(req.Next)
	if err != nil {
		return err
	}
	if err := s.repo.SetPasswordRevokeOtherSessions(ctx, userID, hash, deviceClientID); err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	s.audit.Log(ctx, audit.EventPasswordChanged, userID, ip, "", nil)
	if cur, _, _, err := s.repo.GetEmailState(ctx, userID); err == nil {
		s.notifyAsync(cur, SecurityNotice{Kind: NoticePasswordChanged})
	}
	return nil
}

// RequestEmailChange stores the new address as pending and mails a 6-digit confirmation code to it. The login e-mail only
// switches when that code is entered, so a typo can never lock the owner out and nobody can claim an address they can't read.
func (s *Service) RequestEmailChange(ctx context.Context, userID string, req ChangeEmailRequest, ip string) error {
	newEmail := normalizeEmail(req.NewEmail)
	if newEmail == "" {
		return &ValidationError{Fields: FieldErrors{"email": "enter a valid email address"}}
	}
	if err := s.verifyOwnPassword(ctx, userID, req.Password, "password"); err != nil {
		return err
	}
	current, _, _, err := s.repo.GetEmailState(ctx, userID)
	if err != nil {
		return err
	}
	if newEmail == current {
		return &ValidationError{Fields: FieldErrors{"email": "that is already your email address"}}
	}
	if last, ok, err := s.repo.LastVerificationSentAt(ctx, userID); err != nil {
		return err
	} else if ok && time.Since(last) < verifyResendCooldown {
		return ErrVerifyTooSoon // same 60 s guard as "resend": this endpoint must not become an inbox-spam tool
	}
	if err := s.repo.SetPendingEmail(ctx, userID, newEmail); err != nil {
		if errors.Is(err, ErrEmailTaken) {
			return &ValidationError{Fields: FieldErrors{"email": "an account with this email already exists"}}
		}
		return fmt.Errorf("change email: %w", err)
	}
	s.audit.Log(ctx, audit.EventEmailChangeRequested, userID, ip, "", nil)
	s.notifyAsync(current, SecurityNotice{Kind: NoticeEmailChangeRequired, Detail: maskEmail(newEmail)}) // tell the OLD address
	return s.sendVerification(ctx, userID, newEmail)
}

// DeleteAccount soft-deletes and anonymises the account (see Repository.DeleteAccount) after a password check.
func (s *Service) DeleteAccount(ctx context.Context, userID string, req DeleteAccountRequest, ip string) error {
	if err := s.verifyOwnPassword(ctx, userID, req.Password, "password"); err != nil {
		return err
	}
	if err := s.repo.DeleteAccount(ctx, userID); err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	s.audit.Log(ctx, audit.EventAccountDeleted, userID, ip, "", nil)
	return nil
}

/* ───────── repository ───────── */

func (r *Repository) GetPasswordHash(ctx context.Context, userID string) (string, error) {
	var h string
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(password_hash,'') FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&h)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return h, err
}

// SetPasswordRevokeOtherSessions changes the hash and revokes every session except those of the calling device.
func (r *Repository) SetPasswordRevokeOtherSessions(ctx context.Context, userID, passwordHash, keepDeviceClientID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, passwordHash); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		UPDATE user_sessions SET revoked_at = now(), revoked_reason = 'password_changed'
		WHERE user_id = $1 AND revoked_at IS NULL
		  AND device_id NOT IN (SELECT id FROM user_devices WHERE user_id = $1 AND device_id = $2)
	`, userID, keepDeviceClientID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetPendingEmail records the address waiting for confirmation. Only an address already used by a live account is
// refused here; two people pending the same address is fine, the first confirmation wins (unique index on users.email).
func (r *Repository) SetPendingEmail(ctx context.Context, userID, email string) error {
	var taken bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND deleted_at IS NULL)`, email).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return ErrEmailTaken
	}
	_, err := r.pool.Exec(ctx, `UPDATE users SET pending_email = $2 WHERE id = $1 AND deleted_at IS NULL`, userID, email)
	return err
}

// DeleteAccount soft-deletes the user and scrubs personal data in one transaction. The row stays so existing messages keep a
// valid sender (shown as "Deleted user"); e-mail/username are replaced by unique placeholders so the originals can be re-registered,
// and every credential, session, device, push token, friendship and pending token is removed.
func (r *Repository) DeleteAccount(ctx context.Context, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	stmts := []string{
		`UPDATE users SET deleted_at = now(), status = 'deleted',
		        email = 'deleted-' || id::text || '@deleted.invalid', pending_email = NULL, email_verified_at = NULL,
		        password_hash = NULL, contact_phone = NULL, phone_number = NULL, phone_number_hash = NULL, country_code = NULL,
		        username = 'deleted_' || substr(replace(id::text, '-', ''), 1, 16)
		 WHERE id = $1 AND deleted_at IS NULL`,
		`UPDATE user_profiles SET display_name = 'Deleted user', about = NULL, avatar_media_id = NULL WHERE user_id = $1`,
		`UPDATE user_sessions SET revoked_at = now(), revoked_reason = 'account_deleted' WHERE user_id = $1 AND revoked_at IS NULL`,
		`UPDATE user_devices SET revoked_at = now(), push_token = NULL WHERE user_id = $1 AND revoked_at IS NULL`,
		`DELETE FROM push_tokens WHERE user_id = $1`,
		`DELETE FROM friendships WHERE requester_id = $1 OR addressee_id = $1`,
		`DELETE FROM app_notifications WHERE user_id = $1`,
		`DELETE FROM password_reset_tokens WHERE user_id = $1`,
		`DELETE FROM email_verification_tokens WHERE user_id = $1`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(ctx, q, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

/* ───────── handlers ───────── */

// mountAccountRoutes is called inside the authenticated group (see Routes).
func (h *Handler) mountAccountRoutes(r chi.Router) {
	r.With(httprate.LimitByIP(5, time.Minute)).Post("/change-password", h.changePassword)
	r.With(httprate.LimitByIP(5, time.Minute)).Post("/change-email", h.changeEmail)
	r.With(httprate.LimitByIP(3, time.Minute)).Post("/delete-account", h.deleteAccount)
}

func (h *Handler) writeAccountError(w http.ResponseWriter, r *http.Request, what string, err error) {
	var ve *ValidationError
	switch {
	case errors.As(err, &ve):
		h.validationResponse(w, r, ve)
	case errors.Is(err, ErrVerifyTooSoon):
		response.Error(w, http.StatusTooManyRequests, "too_many_requests", "please wait a minute before trying again", requestID(r))
	default:
		h.log.Error(what+" failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "operation failed", requestID(r))
	}
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var req ChangePasswordRequest
	if !decodeBody(w, r, &req) {
		return
	}
	uid, _ := r.Context().Value(middleware.CtxUserID).(string)
	dev, _ := r.Context().Value(middleware.CtxDeviceID).(string)
	if err := h.svc.ChangePassword(r.Context(), uid, dev, req, clientIP(r)); err != nil {
		h.writeAccountError(w, r, "change password", err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "password_changed"})
}

func (h *Handler) changeEmail(w http.ResponseWriter, r *http.Request) {
	var req ChangeEmailRequest
	if !decodeBody(w, r, &req) {
		return
	}
	uid, _ := r.Context().Value(middleware.CtxUserID).(string)
	if err := h.svc.RequestEmailChange(r.Context(), uid, req, clientIP(r)); err != nil {
		var ve *ValidationError
		if !errors.As(err, &ve) && !errors.Is(err, ErrVerifyTooSoon) {
			// the address is stored as pending but the mail could not be sent: tell the client so it can offer "resend"
			h.log.Error("change email: send failed", zap.Error(err), zap.String("request_id", requestID(r)))
			response.Error(w, http.StatusBadGateway, "mail_failed", "couldn't send the confirmation code right now — use \"Resend code\" to try again", requestID(r))
			return
		}
		h.writeAccountError(w, r, "change email", err)
		return
	}
	response.JSON(w, http.StatusAccepted, map[string]string{"status": "verification_sent"})
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	var req DeleteAccountRequest
	if !decodeBody(w, r, &req) {
		return
	}
	uid, _ := r.Context().Value(middleware.CtxUserID).(string)
	if err := h.svc.DeleteAccount(r.Context(), uid, req, clientIP(r)); err != nil {
		h.writeAccountError(w, r, "delete account", err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
