package chat

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/response"
)

// EnableCalls turns on voice / video / group calls. Without it call events are answered with a "calls_disabled" error.
func (s *Service) EnableCalls(ice IceConfig) {
	s.calls = NewCallService(s.repo, s.hub, ice, s.isBlockedPair)
}

func callErrorCode(err error) (string, string) {
	switch {
	case errors.Is(err, ErrNotAMember):
		return "not_a_member", "you are not a member of this conversation"
	case errors.Is(err, ErrCallBlocked):
		return "call_blocked", "calling is not allowed between these users"
	case errors.Is(err, ErrCallFull):
		return "call_full", "this call is full"
	case errors.Is(err, ErrCallEnded):
		return "call_ended", "this call has already ended"
	case errors.Is(err, ErrCallNotFound):
		return "call_not_found", "call not found"
	case errors.Is(err, ErrCallInvalid):
		return "call_invalid", "invalid call request"
	default:
		return "call_failed", "could not complete the call action"
	}
}

func (h *WSHandler) handleCallEvent(ctx context.Context, c *conn, env Envelope) {
	calls := h.svc.calls
	if calls == nil {
		h.sendError(c, "calls_disabled", "calls are not enabled on this server")
		return
	}
	var err error
	switch env.Type {
	case EventCallStart:
		var p CallStartPayload
		if derr := decodePayload(env.Payload, &p); derr != nil {
			h.sendError(c, "invalid_payload", "malformed call.start payload")
			return
		}
		_, err = calls.Start(ctx, env.ConversationID, c.userID, p.Kind)
	case EventCallJoin:
		var p CallRefPayload
		if derr := decodePayload(env.Payload, &p); derr != nil || p.CallID == "" {
			h.sendError(c, "invalid_payload", "malformed call.join payload")
			return
		}
		_, err = calls.Join(ctx, p.CallID, c.userID)
	case EventCallLeave:
		var p CallRefPayload
		if derr := decodePayload(env.Payload, &p); derr != nil || p.CallID == "" {
			h.sendError(c, "invalid_payload", "malformed call.leave payload")
			return
		}
		err = calls.Leave(ctx, p.CallID, c.userID)
	case EventCallReject:
		var p CallRefPayload
		if derr := decodePayload(env.Payload, &p); derr != nil || p.CallID == "" {
			h.sendError(c, "invalid_payload", "malformed call.reject payload")
			return
		}
		err = calls.Reject(ctx, p.CallID, c.userID)
	case EventCallSignal:
		var p CallSignalPayload
		if derr := decodePayload(env.Payload, &p); derr != nil || p.CallID == "" {
			h.sendError(c, "invalid_payload", "malformed call.signal payload")
			return
		}
		err = calls.Signal(ctx, p.CallID, c.userID, p.ToUserID, p.Data)
	}
	if err != nil {
		code, msg := callErrorCode(err)
		if code == "call_failed" {
			h.log.Warn("call event failed", zap.String("type", env.Type), zap.Error(err))
		}
		h.sendError(c, code, msg)
	}
}

// dropCalls runs when a connection closes: if the user has no other connection left, they leave any live call.
func (h *WSHandler) dropCalls(ctx context.Context, userID string) {
	if h.svc.calls == nil || h.hub.HasLocalConn(userID) {
		return
	}
	h.svc.calls.DropUser(ctx, userID)
}

// listCalls: GET /conversations/{conversationID}/calls?limit=20 — recent calls (the live one, if any, is the newest with status ringing/active).
func (h *Handler) listCalls(w http.ResponseWriter, r *http.Request) {
	if h.svc.calls == nil {
		response.JSON(w, http.StatusOK, map[string]interface{}{"calls": []Call{}})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	calls, err := h.svc.calls.History(r.Context(), chi.URLParam(r, "conversationID"), userID(r), limit)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]interface{}{"calls": calls})
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of this conversation", requestID(r))
	default:
		h.log.Error("list calls failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to load calls", requestID(r))
	}
}
