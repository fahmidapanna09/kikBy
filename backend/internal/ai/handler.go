package ai

import (
	"encoding/json"
	"errors"
	"fmt"
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
	r.Post("/conversations", h.create)
	r.Get("/conversations", h.mine)
	r.Get("/conversations/{conversationID}", h.get)
	r.Patch("/conversations/{conversationID}", h.rename)
	r.Delete("/conversations/{conversationID}", h.delete)
	r.Get("/conversations/{conversationID}/messages", h.listMessages)
	r.Post("/conversations/{conversationID}/messages", h.sendMessage)
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

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateConversationRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // title is optional; malformed/empty body just means "no title"
	c, err := h.svc.CreateConversation(r.Context(), userID(r), req)
	if err != nil {
		h.log.Error("create ai conversation failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to create conversation", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, c)
}

func (h *Handler) mine(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Mine(r.Context(), userID(r))
	if err != nil {
		h.log.Error("list ai conversations failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list conversations", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"conversations": list})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), userID(r), chi.URLParam(r, "conversationID"))
	writeConversationResult(w, r, h, c, err)
}

func (h *Handler) rename(w http.ResponseWriter, r *http.Request) {
	var req UpdateConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	err := h.svc.UpdateTitle(r.Context(), userID(r), chi.URLParam(r, "conversationID"), req)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]any{"ok": true})
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "conversation not found", requestID(r))
	case errors.Is(err, ErrEmptyMessage):
		response.Error(w, http.StatusBadRequest, "invalid_title", "title is required", requestID(r))
	default:
		h.log.Error("rename ai conversation failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to rename conversation", requestID(r))
	}
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Delete(r.Context(), userID(r), chi.URLParam(r, "conversationID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]any{"ok": true})
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "conversation not found", requestID(r))
	default:
		h.log.Error("delete ai conversation failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to delete conversation", requestID(r))
	}
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListMessages(r.Context(), userID(r), chi.URLParam(r, "conversationID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]any{"messages": list})
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "conversation not found", requestID(r))
	default:
		h.log.Error("list ai messages failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list messages", requestID(r))
	}
}

// sendMessage streams the model's reply back as Server-Sent Events:
//
//	event: delta   data: {"text":"..."}        (zero or more, as the model generates)
//	event: done    data: {"message": Message}  (exactly one, on success)
//	event: error   data: {"error":"..."}       (instead of "done", on failure)
//
// SSE (not a custom WS frame) is used here per the spec's API surface
// (`POST /ai/conversations/{id}/messages` — SSE stream) and because it is a
// plain request/response HTTP call, unlike the always-on chat WebSocket.
func (h *Handler) sendMessage(w http.ResponseWriter, r *http.Request) {
	conversationID := chi.URLParam(r, "conversationID")

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}

	flusher, canFlush := w.(http.Flusher)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx response buffering in front of the backend, if any

	headerWritten := false
	writeSSE := func(event string, payload any) {
		if !headerWritten {
			w.WriteHeader(http.StatusOK)
			headerWritten = true
		}
		b, _ := json.Marshal(payload)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		if canFlush {
			flusher.Flush()
		}
	}

	onDelta := func(text string) {
		writeSSE("delta", map[string]string{"text": text})
	}

	msg, err := h.svc.SendMessage(r.Context(), userID(r), conversationID, req, onDelta)
	switch {
	case err == nil:
		writeSSE("done", map[string]any{"message": msg})
	case errors.Is(err, ErrNotFound):
		if !headerWritten {
			response.Error(w, http.StatusNotFound, "not_found", "conversation not found", requestID(r))
			return
		}
		writeSSE("error", map[string]string{"error": "conversation not found"})
	case errors.Is(err, ErrEmptyMessage):
		if !headerWritten {
			response.Error(w, http.StatusBadRequest, "invalid_message", "message content is required", requestID(r))
			return
		}
		writeSSE("error", map[string]string{"error": "message content is required"})
	case errors.Is(err, ErrMessageTooLong):
		if !headerWritten {
			response.Error(w, http.StatusBadRequest, "message_too_long", "message exceeds maximum length", requestID(r))
			return
		}
		writeSSE("error", map[string]string{"error": "message exceeds maximum length"})
	case errors.Is(err, ErrRateLimited):
		if !headerWritten {
			response.Error(w, http.StatusTooManyRequests, "rate_limited", "AI message quota exceeded, try again later", requestID(r))
			return
		}
		writeSSE("error", map[string]string{"error": "rate limited"})
	case errors.Is(err, ErrUpstream):
		// The user's turn was already persisted by the service even though
		// the model call failed — writeSSE("done", ...) is skipped here on
		// purpose since msg carries the recorded error, which the client can
		// still render as a failed assistant bubble.
		writeSSE("done", map[string]any{"message": msg})
	default:
		h.log.Error("ai send message failed", zap.Error(err), zap.String("request_id", requestID(r)))
		if !headerWritten {
			response.Error(w, http.StatusInternalServerError, "internal_error", "failed to generate response", requestID(r))
			return
		}
		writeSSE("error", map[string]string{"error": "internal error"})
	}
}

func writeConversationResult(w http.ResponseWriter, r *http.Request, h *Handler, c Conversation, err error) {
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, c)
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "conversation not found", requestID(r))
	default:
		h.log.Error("ai conversation lookup failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to look up conversation", requestID(r))
	}
}
