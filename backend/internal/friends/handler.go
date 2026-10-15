package friends

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

type Handler struct {
	svc *Service
	log *zap.Logger
}

func NewHandler(svc *Service, log *zap.Logger) *Handler { return &Handler{svc: svc, log: log} }

func (h *Handler) Routes(verifier middleware.TokenVerifier) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(verifier))
	r.Get("/", h.list)
	// Writes are rate-limited per IP on top of the global limit: friend requests are a spam vector.
	r.With(httprate.LimitByIP(30, time.Minute)).Post("/requests", h.sendRequest)
	r.Delete("/requests/{userID}", h.deletePending)
	r.Post("/{userID}/accept", h.accept)
	r.Delete("/{userID}", h.remove)
	r.Put("/{userID}/block", h.block)
	r.Delete("/{userID}/block", h.unblock)
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

// write maps service errors to HTTP. ok is called on success.
func (h *Handler) write(w http.ResponseWriter, r *http.Request, err error, ok func()) {
	switch {
	case err == nil:
		ok()
	case errors.Is(err, ErrInvalidID), errors.Is(err, ErrSelf):
		response.Error(w, http.StatusBadRequest, "invalid_user", err.Error(), requestID(r))
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "not found", requestID(r))
	case errors.Is(err, ErrBlocked):
		response.Error(w, http.StatusConflict, "blocked", "unblock this user first", requestID(r))
	default:
		h.log.Error("friends request failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "something went wrong", requestID(r))
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), userID(r))
	h.write(w, r, err, func() { response.JSON(w, http.StatusOK, map[string]any{"friends": items}) })
}

func (h *Handler) sendRequest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"user_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "user_id is required", requestID(r))
		return
	}
	rel, err := h.svc.SendRequest(r.Context(), userID(r), body.UserID)
	h.write(w, r, err, func() { response.JSON(w, http.StatusOK, rel) })
}

func (h *Handler) accept(w http.ResponseWriter, r *http.Request) {
	rel, err := h.svc.Accept(r.Context(), userID(r), chi.URLParam(r, "userID"))
	h.write(w, r, err, func() { response.JSON(w, http.StatusOK, rel) })
}

func (h *Handler) deletePending(w http.ResponseWriter, r *http.Request) {
	err := h.svc.DeletePending(r.Context(), userID(r), chi.URLParam(r, "userID"))
	h.write(w, r, err, func() { response.JSON(w, http.StatusOK, map[string]string{"status": "removed"}) })
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Remove(r.Context(), userID(r), chi.URLParam(r, "userID"))
	h.write(w, r, err, func() { response.JSON(w, http.StatusOK, map[string]string{"status": "removed"}) })
}

func (h *Handler) block(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Block(r.Context(), userID(r), chi.URLParam(r, "userID"))
	h.write(w, r, err, func() { response.JSON(w, http.StatusOK, map[string]string{"status": "blocked"}) })
}

func (h *Handler) unblock(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Unblock(r.Context(), userID(r), chi.URLParam(r, "userID"))
	h.write(w, r, err, func() { response.JSON(w, http.StatusOK, map[string]string{"status": "unblocked"}) })
}
