package inbox

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	r.Post("/read-all", h.readAll)
	r.Post("/{id}/read", h.read)
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

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, msg string, err error) {
	h.log.Error(msg, zap.Error(err), zap.String("request_id", requestID(r)))
	response.Error(w, http.StatusInternalServerError, "internal_error", msg, requestID(r))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, unread, err := h.svc.List(r.Context(), userID(r))
	if err != nil {
		h.fail(w, r, "failed to load notifications", err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"notifications": items, "unread_count": unread})
}

func (h *Handler) read(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		response.Error(w, http.StatusNotFound, "not_found", "notification not found", requestID(r))
		return
	}
	err := h.svc.MarkRead(r.Context(), userID(r), id)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "read"})
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "notification not found", requestID(r))
	default:
		h.fail(w, r, "failed to update notification", err)
	}
}

func (h *Handler) readAll(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.MarkAllRead(r.Context(), userID(r)); err != nil {
		h.fail(w, r, "failed to update notifications", err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "all_read"})
}
