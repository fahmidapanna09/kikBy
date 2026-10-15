package chat

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

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
	r.Get("/conversations", h.listConversations)
	r.Post("/conversations", h.startDirectConversation)
	r.Get("/conversations/{conversationID}/messages", h.listMessages)
	r.Post("/conversations/{conversationID}/messages", h.sendMessage) // fallback path if WS is unavailable
	r.Put("/conversations/{conversationID}/mute", h.setMute)

	r.Post("/groups", h.createGroup)
	r.Get("/groups/{conversationID}", h.getGroup)
	r.Patch("/groups/{conversationID}", h.updateGroup)
	r.Post("/groups/{conversationID}/members", h.addMembers)
	r.Delete("/groups/{conversationID}/members/{userID}", h.removeMember)
	r.Put("/groups/{conversationID}/members/{userID}/role", h.updateMemberRole)
	r.Post("/groups/{conversationID}/invite-link", h.generateInviteLink)
	r.Delete("/groups/{conversationID}/invite-link", h.revokeInviteLink)
	r.Post("/groups/join/{inviteCode}", h.joinViaInvite)

	r.Post("/messages/{messageID}/reactions", h.setReaction)
	r.Delete("/messages/{messageID}/reactions", h.removeReaction)
	r.Get("/messages/{messageID}/reactions", h.listReactions)

	r.Get("/search/messages", h.searchMessages)
	h.mountWebRoutes(r)
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

func deviceID(r *http.Request) string {
	id, _ := r.Context().Value(middleware.CtxDeviceID).(string)
	return id
}

func (h *Handler) listConversations(w http.ResponseWriter, r *http.Request) {
	convos, err := h.svc.ListConversations(r.Context(), userID(r))
	if err != nil {
		h.log.Error("list conversations failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list conversations", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"conversations": convos})
}

func (h *Handler) startDirectConversation(w http.ResponseWriter, r *http.Request) {
	var req StartDirectConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PeerUserID == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "peer_user_id is required", requestID(r))
		return
	}
	if req.PeerUserID == userID(r) {
		response.Error(w, http.StatusBadRequest, "invalid_peer", "cannot start a conversation with yourself", requestID(r))
		return
	}
	convo, err := h.svc.StartDirectConversation(r.Context(), userID(r), req.PeerUserID)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, ConversationSummary{ID: convo.ID, Type: convo.Type, PeerUserID: req.PeerUserID})
	case errors.Is(err, ErrBlocked):
		response.Error(w, http.StatusForbidden, "blocked", "you can't message this user", requestID(r))
	default:
		h.log.Error("start direct conversation failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to start conversation", requestID(r))
	}
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	conversationID := chi.URLParam(r, "conversationID")
	var beforeSeq int64
	if c := r.URL.Query().Get("cursor"); c != "" {
		n, err := strconv.ParseInt(c, 10, 64)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid_cursor", "cursor must be an integer seq", requestID(r))
			return
		}
		beforeSeq = n
	}

	page, err := h.svc.ListMessages(r.Context(), userID(r), conversationID, beforeSeq)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, page)
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of this conversation", requestID(r))
	default:
		h.log.Error("list messages failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list messages", requestID(r))
	}
}

// sendMessage is a REST fallback for sending when the WebSocket is down;
// the primary path is the WS message.send event (ws_handler.go), which also
// gives the sender an immediate ack over the same connection. This endpoint
// exists so the Android offline-queue can flush pending messages over plain
// HTTPS if reconnecting the socket is taking too long.
func (h *Handler) sendMessage(w http.ResponseWriter, r *http.Request) {
	conversationID := chi.URLParam(r, "conversationID")
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ClientTmpID == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "client_tmp_id is required", requestID(r))
		return
	}

	msg, err := h.svc.SendMessage(r.Context(), conversationID, userID(r), deviceID(r), req)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, msg)
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of this conversation", requestID(r))
	case errors.Is(err, ErrSendNotAllowed):
		response.Error(w, http.StatusForbidden, "send_not_allowed", "only admins can send messages in this group", requestID(r))
	case errors.Is(err, ErrBlocked):
		response.Error(w, http.StatusForbidden, "blocked", "you can't message this user", requestID(r))
	default:
		h.log.Error("send message (rest fallback) failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to send message", requestID(r))
	}
}
