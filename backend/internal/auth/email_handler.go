package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/middleware"
	"github.com/yourorg/chatplatform/internal/platform/response"
)

// mountEmailRoutes adds /register, /login, /forgot-password, /reset-password (all public).
// Each has its own per-IP limit on top of the global one (credential endpoints
// are the brute-force surface).
func (h *Handler) mountEmailRoutes(r chi.Router) {
	r.With(httprate.LimitByIP(10, time.Minute)).Post("/register", h.register)
	r.With(httprate.LimitByIP(10, time.Minute)).Post("/login", h.login)
	r.With(httprate.LimitByIP(5, time.Minute)).Post("/forgot-password", h.forgotPassword)
	r.With(httprate.LimitByIP(10, time.Minute)).Post("/reset-password", h.resetPassword)
}

// mountVerifyResend is called inside the authenticated group (see Routes). Entering the e-mailed 6-digit code needs a
// signed-in session on purpose: the code is checked only against THIS user's newest code, which is what makes the
// 5-tries limit meaningful (a public endpoint could be hammered across many accounts).
func (h *Handler) mountVerifyResend(r chi.Router) {
	r.With(httprate.LimitByIP(5, time.Minute)).Post("/verify-email/resend", h.resendVerification)
	r.With(httprate.LimitByIP(10, time.Minute)).Post("/verify-email", h.verifyEmail)
}

const maxAuthBody = 8 << 10 // 8 KiB is far more than any of these payloads need

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBody)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return false
	}
	return true
}

func (h *Handler) validationResponse(w http.ResponseWriter, r *http.Request, ve *ValidationError) {
	// response.Error has no field map, so emit the same envelope shape plus "fields".
	response.JSON(w, http.StatusUnprocessableEntity, map[string]any{
		"error": map[string]any{
			"code": "validation", "message": "please fix the highlighted fields",
			"request_id": requestID(r), "fields": ve.Fields,
		},
	})
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.DeviceID == "" || req.Platform == "" {
		response.Error(w, http.StatusBadRequest, "missing_device_info", "device_id and platform are required", requestID(r))
		return
	}
	res, err := h.svc.Register(r.Context(), req, clientIP(r), r.UserAgent())
	var ve *ValidationError
	switch {
	case err == nil:
		response.JSON(w, http.StatusCreated, res)
	case errors.As(err, &ve):
		h.validationResponse(w, r, ve)
	default:
		h.log.Error("register failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to create account", requestID(r))
	}
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Identifier == "" || req.Password == "" || req.DeviceID == "" || req.Platform == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "identifier, password, device_id and platform are required", requestID(r))
		return
	}
	res, err := h.svc.Login(r.Context(), req, clientIP(r), r.UserAgent())
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrInvalidCredentials):
		response.Error(w, http.StatusUnauthorized, "invalid_credentials", "incorrect email/username or password", requestID(r))
	case errors.Is(err, ErrAccountSuspended):
		response.Error(w, http.StatusForbidden, "account_suspended", "this account has been suspended", requestID(r))
	case errors.Is(err, ErrAccountLocked):
		w.Header().Set("Retry-After", "900")
		response.Error(w, http.StatusTooManyRequests, "account_locked", "Too many failed attempts. Try again in 15 minutes, or reset your password.", requestID(r))
	default:
		h.log.Error("login failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to sign in", requestID(r))
	}
}

func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), req.Email, clientIP(r)); err != nil {
		// Logged, but the client always gets the same neutral answer.
		h.log.Error("forgot password failed", zap.Error(err), zap.String("request_id", requestID(r)))
	}
	response.JSON(w, http.StatusAccepted, map[string]string{"status": "if_account_exists_email_sent"})
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if !decodeBody(w, r, &req) {
		return
	}
	err := h.svc.ResetPassword(r.Context(), req, clientIP(r))
	var ve *ValidationError
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "password_updated"})
	case errors.As(err, &ve):
		h.validationResponse(w, r, ve)
	case errors.Is(err, ErrTooManyCodeAttempts):
		response.Error(w, http.StatusTooManyRequests, "too_many_attempts", "Too many wrong codes. Request a new code and try again.", requestID(r))
	case errors.Is(err, ErrInvalidResetToken):
		response.Error(w, http.StatusBadRequest, "invalid_code", "That code is wrong or has expired.", requestID(r))
	default:
		h.log.Error("reset password failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to reset password", requestID(r))
	}
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req VerifyEmailRequest
	if !decodeBody(w, r, &req) {
		return
	}
	uid, _ := r.Context().Value(middleware.CtxUserID).(string)
	switch err := h.svc.VerifyEmailCode(r.Context(), uid, req.Code, clientIP(r)); {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "verified"})
	case errors.Is(err, ErrInvalidVerifyCode):
		response.Error(w, http.StatusBadRequest, "invalid_code", "that code isn't right — check it and try again", requestID(r))
	case errors.Is(err, ErrVerifyCodeExpired):
		response.Error(w, http.StatusBadRequest, "code_expired", "this code has expired — request a new one", requestID(r))
	case errors.Is(err, ErrTooManyCodeAttempts):
		response.Error(w, http.StatusTooManyRequests, "too_many_attempts", "too many wrong codes — request a new one", requestID(r))
	default:
		h.log.Error("verify email failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to verify e-mail", requestID(r))
	}
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(middleware.CtxUserID).(string)
	already, err := h.svc.ResendVerification(r.Context(), uid)
	switch {
	case err == nil && already:
		response.JSON(w, http.StatusOK, map[string]string{"status": "already_verified"})
	case err == nil:
		response.JSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
	case errors.Is(err, ErrVerifyTooSoon):
		response.Error(w, http.StatusTooManyRequests, "too_many_requests", "a code was just sent — wait a minute before asking for another", requestID(r))
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusBadRequest, "no_email", "this account has no e-mail address to verify", requestID(r))
	default:
		h.log.Error("resend verification failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusBadGateway, "mail_failed", "couldn't send the e-mail right now, please try again", requestID(r))
	}
}
