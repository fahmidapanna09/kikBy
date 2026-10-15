package chat

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/response"
)

// mountWebRoutes adds the endpoints the web client needs on top of the
// original REST surface. Kept in its own file so merging with local edits to
// handler.go is a one-line change (the call in Routes).
func (h *Handler) mountWebRoutes(r chi.Router) {
	r.Post("/conversations/{conversationID}/read", h.markRead)
	r.Put("/conversations/{conversationID}/pin", h.setPinned)
	r.Put("/conversations/{conversationID}/archive", h.setArchived)
	r.Patch("/messages/{messageID}", h.editMessage)
	r.Delete("/messages/{messageID}", h.deleteMessage)
	r.Get("/conversations/{conversationID}/calls", h.listCalls)
}

func (h *Handler) writeMessageError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "message not found", requestID(r))
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of this conversation", requestID(r))
	case errors.Is(err, ErrNotSender):
		response.Error(w, http.StatusForbidden, "not_sender", "only the sender can do that", requestID(r))
	case errors.Is(err, ErrMessageDeleted):
		response.Error(w, http.StatusConflict, "message_deleted", "this message was deleted", requestID(r))
	case errors.Is(err, ErrNotEditable):
		response.Error(w, http.StatusBadRequest, "not_editable", "only text messages can be edited", requestID(r))
	case errors.Is(err, ErrInvalidBody):
		response.Error(w, http.StatusBadRequest, "invalid_body", "message text is empty or too long", requestID(r))
	default:
		h.log.Error("message action failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "operation failed", requestID(r))
	}
}

func (h *Handler) editMessage(w http.ResponseWriter, r *http.Request) {
	var req EditMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	msg, err := h.svc.EditMessage(r.Context(), userID(r), chi.URLParam(r, "messageID"), req.Body)
	if err != nil {
		h.writeMessageError(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, msg)
}

// deleteMessage: ?scope=everyone (sender only) or ?scope=me (default).
func (h *Handler) deleteMessage(w http.ResponseWriter, r *http.Request) {
	everyone := r.URL.Query().Get("scope") == "everyone"
	if err := h.svc.DeleteMessage(r.Context(), userID(r), chi.URLParam(r, "messageID"), everyone); err != nil {
		h.writeMessageError(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	var req MarkReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MessageID == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "message_id is required", requestID(r))
		return
	}
	err := h.svc.MarkRead(r.Context(), chi.URLParam(r, "conversationID"), req.MessageID, userID(r), 0)
	writeGroupActionResult(w, r, h, err)
}

func (h *Handler) setPinned(w http.ResponseWriter, r *http.Request) {
	var req FlagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	writeGroupActionResult(w, r, h, h.svc.SetPinned(r.Context(), userID(r), chi.URLParam(r, "conversationID"), req.Value))
}

func (h *Handler) setArchived(w http.ResponseWriter, r *http.Request) {
	var req FlagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	writeGroupActionResult(w, r, h, h.svc.SetArchived(r.Context(), userID(r), chi.URLParam(r, "conversationID"), req.Value))
}
