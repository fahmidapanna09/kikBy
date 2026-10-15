package users

import (
	"encoding/json"
	"errors"
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

func (h *Handler) Routes(verifier middleware.TokenVerifier) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(verifier))
	r.Get("/me", h.getMe)
	r.Patch("/me", h.updateMe)
	r.Patch("/username", h.updateUsername)
	r.Get("/me/settings", h.getSettings)
	r.Put("/me/settings", h.putSettings)
	r.Get("/search", h.search)
	r.Get("/{userID}", h.getPublicProfile)
	r.Post("/{userID}/report", h.reportUser)
	return r
}

func requestID(r *http.Request) string {
	id, _ := r.Context().Value(middleware.CtxRequestID).(string)
	return id
}
func userID(r *http.Request) string {
	id, _ := r.Context().Value(middleware.CtxUserID).(string)
	return id
}

func (h *Handler) getMe(w http.ResponseWriter, r *http.Request) {
	me, err := h.svc.GetMe(r.Context(), userID(r))
	if err != nil {
		h.log.Error("get me failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to load profile", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, me)
}

func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) {
	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	me, err := h.svc.UpdateProfile(r.Context(), userID(r), req)
	if err != nil {
		h.log.Error("update profile failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusBadRequest, "invalid_update", err.Error(), requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, me)
}

func (h *Handler) updateUsername(w http.ResponseWriter, r *http.Request) {
	var req UpdateUsernameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "username is required", requestID(r))
		return
	}
	me, err := h.svc.UpdateUsername(r.Context(), userID(r), req.Username)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, me)
	case errors.Is(err, ErrInvalidUsername):
		response.Error(w, http.StatusBadRequest, "invalid_username", err.Error(), requestID(r))
	case errors.Is(err, ErrUsernameTaken):
		response.Error(w, http.StatusConflict, "username_taken", "that username is already taken", requestID(r))
	default:
		h.log.Error("update username failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to update username", requestID(r))
	}
}

func (h *Handler) getPublicProfile(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	profile, err := h.svc.GetPublicProfile(r.Context(), targetID)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, profile)
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "user not found", requestID(r))
	default:
		h.log.Error("get public profile failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to load profile", requestID(r))
	}
}

// search handles both ?username= (prefix search) and ?phone= (exact match,
// tightly rate-limited) — see service.go for why they have very different
// rate limits despite sharing one endpoint.
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	if username := r.URL.Query().Get("username"); username != "" {
		res, err := h.svc.SearchByUsername(r.Context(), userID(r), username)
		h.writeSearchResult(w, r, res, err)
		return
	}
	if phone := r.URL.Query().Get("phone"); phone != "" {
		if !isValidE164(phone) {
			response.Error(w, http.StatusBadRequest, "invalid_phone_number", "phone must be in E.164 format", requestID(r))
			return
		}
		res, err := h.svc.SearchByPhone(r.Context(), userID(r), phone)
		h.writeSearchResult(w, r, res, err)
		return
	}
	response.Error(w, http.StatusBadRequest, "missing_query", "provide either ?username= or ?phone=", requestID(r))
}

func (h *Handler) writeSearchResult(w http.ResponseWriter, r *http.Request, res SearchResponse, err error) {
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrRateLimited):
		response.Error(w, http.StatusTooManyRequests, "rate_limited", "too many searches, try again later", requestID(r))
	default:
		h.log.Error("search failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "search failed", requestID(r))
	}
}

func (h *Handler) reportUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	var req ReportUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Reason == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "reason is required", requestID(r))
		return
	}
	var contextMsgID *string
	if req.ContextMessageID != "" {
		contextMsgID = &req.ContextMessageID
	}
	err := h.svc.ReportUser(r.Context(), userID(r), targetID, req.Reason, contextMsgID)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "reported"})
	case errors.Is(err, ErrCannotReportSelf):
		response.Error(w, http.StatusBadRequest, "cannot_report_self", "you cannot report yourself", requestID(r))
	default:
		h.log.Error("report user failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to submit report", requestID(r))
	}
}

// isValidE164 mirrors auth's phone validation (kept as a small local copy
// rather than importing the auth package here, to avoid users depending on
// auth for one regex — see architecture doc's module-boundary rule).
var isValidE164 = func(phone string) bool {
	if len(phone) < 8 || len(phone) > 16 || phone[0] != '+' {
		return false
	}
	for _, c := range phone[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return phone[1] != '0'
}
