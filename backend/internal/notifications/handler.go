package notifications

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/middleware"
	"github.com/yourorg/chatplatform/internal/platform/response"
)

// Handler lets the signed-in app register (or drop) the phone's FCM token.
type Handler struct {
	tokens *TokenRepository
	log    *zap.Logger
}

func NewHandler(tokens *TokenRepository, log *zap.Logger) *Handler {
	return &Handler{tokens: tokens, log: log}
}

func (h *Handler) Routes(verifier middleware.TokenVerifier) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(verifier))
	r.Put("/token", h.put)
	r.Delete("/token", h.del)
	return r
}

func reqID(r *http.Request) string {
	id, _ := r.Context().Value(middleware.CtxRequestID).(string)
	return id
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "invalid JSON body", reqID(r))
		return
	}
	tok := strings.TrimSpace(body.Token)
	if len(tok) < 20 || len(tok) > 4096 {
		response.Error(w, http.StatusBadRequest, "invalid_token", "push token looks wrong", reqID(r))
		return
	}
	userID, _ := r.Context().Value(middleware.CtxUserID).(string)
	deviceRow, _ := r.Context().Value(middleware.CtxDeviceID).(string)
	if err := h.tokens.RegisterToken(r.Context(), userID, deviceRow, tok); err != nil {
		h.log.Error("push: register token failed", zap.Error(err), zap.String("request_id", reqID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to save push token", reqID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

func (h *Handler) del(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.CtxUserID).(string)
	deviceRow, _ := r.Context().Value(middleware.CtxDeviceID).(string)
	if err := h.tokens.DeleteForDevice(r.Context(), userID, deviceRow); err != nil {
		h.log.Error("push: delete token failed", zap.Error(err), zap.String("request_id", reqID(r)))
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}
