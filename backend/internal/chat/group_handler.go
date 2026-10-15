package chat

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/response"
)

// --- Groups ---

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "name is required", requestID(r))
		return
	}
	group, err := h.svc.CreateGroup(r.Context(), userID(r), req)
	if err != nil {
		h.log.Error("create group failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to create group", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, group)
}

func (h *Handler) getGroup(w http.ResponseWriter, r *http.Request) {
	conversationID := chi.URLParam(r, "conversationID")
	group, err := h.svc.GetGroup(r.Context(), userID(r), conversationID)
	writeGroupResult(w, r, h, group, err)
}

func (h *Handler) updateGroup(w http.ResponseWriter, r *http.Request) {
	conversationID := chi.URLParam(r, "conversationID")
	var req UpdateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	group, err := h.svc.UpdateGroup(r.Context(), userID(r), conversationID, req)
	writeGroupResult(w, r, h, group, err)
}

func writeGroupResult(w http.ResponseWriter, r *http.Request, h *Handler, group GroupSummary, err error) {
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, group)
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of this group", requestID(r))
	case errors.Is(err, ErrGroupNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "group not found", requestID(r))
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "permission_denied", "you don't have permission to do that", requestID(r))
	default:
		h.log.Error("group operation failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "operation failed", requestID(r))
	}
}

func (h *Handler) addMembers(w http.ResponseWriter, r *http.Request) {
	conversationID := chi.URLParam(r, "conversationID")
	var req AddMembersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.UserIDs) == 0 {
		response.Error(w, http.StatusBadRequest, "invalid_body", "user_ids is required", requestID(r))
		return
	}
	err := h.svc.AddMembers(r.Context(), userID(r), conversationID, req.UserIDs)
	writeGroupActionResult(w, r, h, err)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	conversationID := chi.URLParam(r, "conversationID")
	targetUserID := chi.URLParam(r, "userID")
	err := h.svc.RemoveMember(r.Context(), userID(r), conversationID, targetUserID)
	writeGroupActionResult(w, r, h, err)
}

func (h *Handler) updateMemberRole(w http.ResponseWriter, r *http.Request) {
	conversationID := chi.URLParam(r, "conversationID")
	targetUserID := chi.URLParam(r, "userID")
	var req UpdateMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Role == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "role is required", requestID(r))
		return
	}
	err := h.svc.UpdateMemberRole(r.Context(), userID(r), conversationID, targetUserID, req.Role)
	writeGroupActionResult(w, r, h, err)
}

func writeGroupActionResult(w http.ResponseWriter, r *http.Request, h *Handler, err error) {
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of this group", requestID(r))
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "permission_denied", "you don't have permission to do that", requestID(r))
	default:
		h.log.Error("group action failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "operation failed", requestID(r))
	}
}

func (h *Handler) generateInviteLink(w http.ResponseWriter, r *http.Request) {
	conversationID := chi.URLParam(r, "conversationID")
	code, err := h.svc.GenerateInviteLink(r.Context(), userID(r), conversationID)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, InviteLinkResponse{InviteCode: code})
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of this group", requestID(r))
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "permission_denied", "only admins can generate an invite link", requestID(r))
	default:
		h.log.Error("generate invite link failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to generate invite link", requestID(r))
	}
}

func (h *Handler) revokeInviteLink(w http.ResponseWriter, r *http.Request) {
	conversationID := chi.URLParam(r, "conversationID")
	err := h.svc.RevokeInviteLink(r.Context(), userID(r), conversationID)
	writeGroupActionResult(w, r, h, err)
}

func (h *Handler) joinViaInvite(w http.ResponseWriter, r *http.Request) {
	inviteCode := chi.URLParam(r, "inviteCode")
	conversationID, err := h.svc.JoinViaInvite(r.Context(), userID(r), inviteCode)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, JoinByInviteResponse{ConversationID: conversationID})
	case errors.Is(err, ErrInviteNotFound):
		response.Error(w, http.StatusNotFound, "invite_not_found", "this invite link is invalid or has been revoked", requestID(r))
	default:
		h.log.Error("join via invite failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to join group", requestID(r))
	}
}

// --- Mute ---

func (h *Handler) setMute(w http.ResponseWriter, r *http.Request) {
	conversationID := chi.URLParam(r, "conversationID")
	var req MuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	var until *time.Time
	if req.UntilRFC3339 != "" {
		t, err := time.Parse(time.RFC3339, req.UntilRFC3339)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid_until", "until must be RFC3339, or omitted to unmute", requestID(r))
			return
		}
		until = &t
	}
	err := h.svc.SetMute(r.Context(), userID(r), conversationID, until)
	writeGroupActionResult(w, r, h, err)
}

// --- Reactions ---

func (h *Handler) setReaction(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageID")
	var req ReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Emoji == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "emoji is required", requestID(r))
		return
	}
	err := h.svc.SetReaction(r.Context(), userID(r), messageID, req.Emoji)
	writeGroupActionResult(w, r, h, err)
}

func (h *Handler) removeReaction(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageID")
	err := h.svc.RemoveReaction(r.Context(), userID(r), messageID)
	writeGroupActionResult(w, r, h, err)
}

func (h *Handler) listReactions(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageID")
	reactions, err := h.svc.ListReactions(r.Context(), userID(r), messageID)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]any{"reactions": reactions})
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of this conversation", requestID(r))
	default:
		h.log.Error("list reactions failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list reactions", requestID(r))
	}
}

// --- Search ---

func (h *Handler) searchMessages(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	var conversationID *string
	if c := r.URL.Query().Get("conversation_id"); c != "" {
		conversationID = &c
	}
	result, err := h.svc.SearchMessages(r.Context(), userID(r), query, conversationID)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, result)
	case errors.Is(err, ErrEmptySearchQuery):
		response.Error(w, http.StatusBadRequest, "empty_query", "q must not be empty", requestID(r))
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of that conversation", requestID(r))
	default:
		h.log.Error("search messages failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "search failed", requestID(r))
	}
}
