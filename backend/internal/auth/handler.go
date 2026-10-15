package auth

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/middleware"
	"github.com/yourorg/chatplatform/internal/platform/response"
)

type Handler struct {
	svc *Service
	log *zap.Logger
}

func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Routes wires the always-on production routes. Call MountDebugRoutes
// separately, and only when APP_ENV=local, to add the OTP-peek endpoint.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/otp/request", h.requestOTP)
	r.Post("/otp/verify", h.verifyOTP)
	r.Post("/refresh", h.refresh)
	h.mountEmailRoutes(r)

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(h.svc))
		r.Post("/logout", h.logout)
		h.mountVerifyResend(r)
		h.mountAccountRoutes(r)
		r.Get("/sessions", h.listSessions)
		r.Delete("/sessions/{sessionID}", h.revokeSession)
		r.Post("/sessions/revoke-all", h.revokeAllSessions)
	})
	return r
}

// MountDebugRoutes adds an OTP-peek endpoint so a developer can log in
// end-to-end without a real SMS provider. The caller (see cmd/api/main.go)
// MUST only invoke this when APP_ENV=local — never in staging/production —
// since it lets anyone who can reach the API read a live OTP for any phone
// number.
func (h *Handler) MountDebugRoutes(r chi.Router) {
	r.Get("/debug/otp/{phoneNumber}", func(w http.ResponseWriter, r *http.Request) {
		phone := chi.URLParam(r, "phoneNumber")
		code, err := h.svc.DebugPeekOTP(r.Context(), phone)
		if err != nil {
			response.Error(w, http.StatusNotFound, "no_otp_pending", "no otp pending for this number", requestID(r))
			return
		}
		response.JSON(w, http.StatusOK, map[string]string{"code": code})
	})
}

func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	// In production, terminate TLS at a load balancer that sets X-Forwarded-For
	// and trust only that proxy's hop — left as a TODO wired at deploy time.
	return ip
}

func requestID(r *http.Request) string {
	id, _ := r.Context().Value(middleware.CtxRequestID).(string)
	return id
}

func (h *Handler) requestOTP(w http.ResponseWriter, r *http.Request) {
	var req RequestOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	if req.Purpose == "" {
		req.Purpose = "login"
	}

	err := h.svc.RequestOTP(r.Context(), req.PhoneNumber, clientIP(r))
	switch {
	case err == nil:
		response.JSON(w, http.StatusAccepted, map[string]string{"status": "otp_sent"})
	case errors.Is(err, ErrInvalidPhoneNumber):
		response.Error(w, http.StatusBadRequest, "invalid_phone_number", "phone number must be in E.164 format", requestID(r))
	case errors.Is(err, ErrOTPRateLimited):
		response.Error(w, http.StatusTooManyRequests, "rate_limited", "too many OTP requests, try again later", requestID(r))
	default:
		h.log.Error("request otp failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to send otp", requestID(r))
	}
}

func (h *Handler) verifyOTP(w http.ResponseWriter, r *http.Request) {
	var req VerifyOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	if req.DeviceID == "" || req.Platform == "" {
		response.Error(w, http.StatusBadRequest, "missing_device_info", "device_id and platform are required", requestID(r))
		return
	}

	res, err := h.svc.VerifyOTP(r.Context(), req, clientIP(r), r.UserAgent())
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrInvalidPhoneNumber):
		response.Error(w, http.StatusBadRequest, "invalid_phone_number", "phone number must be in E.164 format", requestID(r))
	case errors.Is(err, ErrOTPInvalid):
		response.Error(w, http.StatusUnauthorized, "invalid_otp", "code is invalid or expired", requestID(r))
	case errors.Is(err, ErrAccountSuspended):
		response.Error(w, http.StatusForbidden, "account_suspended", "this account has been suspended", requestID(r))
	default:
		h.log.Error("verify otp failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to verify otp", requestID(r))
	}
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "refresh_token is required", requestID(r))
		return
	}

	tokens, err := h.svc.Refresh(r.Context(), req.RefreshToken, clientIP(r), r.UserAgent())
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, tokens)
	case errors.Is(err, ErrSessionExpired):
		response.Error(w, http.StatusUnauthorized, "session_expired", "session expired or revoked, please log in again", requestID(r))
	case errors.Is(err, ErrRefreshTokenReused):
		h.log.Warn("refresh token reuse detected", zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusUnauthorized, "token_reuse_detected", "security check failed, please log in again", requestID(r))
	default:
		h.log.Error("refresh failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to refresh session", requestID(r))
	}
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "refresh_token is required", requestID(r))
		return
	}
	if err := h.svc.Logout(r.Context(), req.RefreshToken, clientIP(r), r.UserAgent()); err != nil {
		h.log.Error("logout failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to logout", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.CtxUserID).(string)
	sessions, err := h.svc.ListSessions(r.Context(), userID)
	if err != nil {
		h.log.Error("list sessions failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list sessions", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (h *Handler) revokeSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	userID, _ := r.Context().Value(middleware.CtxUserID).(string)

	err := h.svc.RevokeSession(r.Context(), sessionID, userID, clientIP(r), r.UserAgent())
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "revoked"})
	case errors.Is(err, ErrNotFound):
		// Deliberately the same 404 whether the session doesn't exist or
		// belongs to someone else — don't leak which case it was.
		response.Error(w, http.StatusNotFound, "session_not_found", "session not found", requestID(r))
	default:
		h.log.Error("revoke session failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to revoke session", requestID(r))
	}
}

// revokeAllSessions is the "log out everywhere" / "I think my account is
// compromised" endpoint — see Service.RevokeAllSessions for what it does
// and does not guarantee about the caller's own in-flight access token.
func (h *Handler) revokeAllSessions(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.CtxUserID).(string)
	if err := h.svc.RevokeAllSessions(r.Context(), userID, clientIP(r), r.UserAgent()); err != nil {
		h.log.Error("revoke all sessions failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to revoke sessions", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "all_revoked"})
}
