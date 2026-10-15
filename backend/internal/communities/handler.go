package communities

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
	r.Post("/", h.create)
	r.Get("/search", h.search)
	r.Get("/mine", h.mine)
	r.Get("/{communityID}", h.get)
	r.Patch("/{communityID}", h.update)
	r.Post("/{communityID}/join", h.joinDiscoverable)
	r.Post("/join/{inviteCode}", h.joinByInvite)
	r.Post("/{communityID}/leave", h.leave)
	r.Get("/{communityID}/members", h.listMembers)
	r.Put("/{communityID}/members/{userID}/role", h.updateMemberRole)
	r.Delete("/{communityID}/members/{userID}", h.removeMember)
	r.Post("/{communityID}/ban/{userID}", h.ban)
	r.Delete("/{communityID}/ban/{userID}", h.unban)
	r.Post("/{communityID}/groups", h.createGroup)
	r.Post("/{communityID}/groups/{conversationID}", h.addExistingGroup)
	r.Delete("/{communityID}/groups/{conversationID}", h.removeGroup)
	r.Post("/{communityID}/invite-link", h.generateInviteLink)
	r.Delete("/{communityID}/invite-link", h.revokeInviteLink)
	r.Post("/{communityID}/report", h.report)
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
	var req CreateCommunityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	c, err := h.svc.CreateCommunity(r.Context(), userID(r), req)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, c)
	case errors.Is(err, ErrEmptyName):
		response.Error(w, http.StatusBadRequest, "invalid_name", "name is required", requestID(r))
	default:
		h.log.Error("create community failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to create community", requestID(r))
	}
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), userID(r), chi.URLParam(r, "communityID"))
	writeCommunityResult(w, r, h, c, err)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateCommunityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	c, err := h.svc.Update(r.Context(), userID(r), chi.URLParam(r, "communityID"), req)
	writeCommunityResult(w, r, h, c, err)
}

func writeCommunityResult(w http.ResponseWriter, r *http.Request, h *Handler, c CommunitySummary, err error) {
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, c)
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "community not found", requestID(r))
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of this community", requestID(r))
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "permission_denied", "you don't have permission to do that", requestID(r))
	case errors.Is(err, ErrEmptyName):
		response.Error(w, http.StatusBadRequest, "invalid_name", "name is required", requestID(r))
	default:
		h.log.Error("community operation failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "operation failed", requestID(r))
	}
}

func (h *Handler) joinDiscoverable(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.JoinDiscoverable(r.Context(), userID(r), chi.URLParam(r, "communityID"))
	writeJoinResult(w, r, h, res, err)
}

func (h *Handler) joinByInvite(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.JoinByInvite(r.Context(), userID(r), chi.URLParam(r, "inviteCode"))
	switch {
	case errors.Is(err, ErrInviteNotFound):
		response.Error(w, http.StatusNotFound, "invite_not_found", "this invite link is invalid or has been revoked", requestID(r))
	default:
		writeJoinResult(w, r, h, res, err)
	}
}

func writeJoinResult(w http.ResponseWriter, r *http.Request, h *Handler, res JoinResponse, err error) {
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "community not found", requestID(r))
	case errors.Is(err, ErrBanned):
		response.Error(w, http.StatusForbidden, "banned", "you have been banned from this community", requestID(r))
	default:
		h.log.Error("join community failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to join community", requestID(r))
	}
}

func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Leave(r.Context(), userID(r), chi.URLParam(r, "communityID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	members, err := h.svc.ListMembers(r.Context(), userID(r), chi.URLParam(r, "communityID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]any{"members": members})
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of this community", requestID(r))
	default:
		h.log.Error("list members failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list members", requestID(r))
	}
}

func (h *Handler) updateMemberRole(w http.ResponseWriter, r *http.Request) {
	var req UpdateMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Role == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "role is required", requestID(r))
		return
	}
	err := h.svc.UpdateMemberRole(r.Context(), userID(r), chi.URLParam(r, "communityID"), chi.URLParam(r, "userID"), req.Role)
	writeActionResult(w, r, h, err)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	err := h.svc.RemoveMember(r.Context(), userID(r), chi.URLParam(r, "communityID"), chi.URLParam(r, "userID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) ban(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Ban(r.Context(), userID(r), chi.URLParam(r, "communityID"), chi.URLParam(r, "userID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) unban(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Unban(r.Context(), userID(r), chi.URLParam(r, "communityID"), chi.URLParam(r, "userID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	var req CreateGroupInCommunityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "name is required", requestID(r))
		return
	}
	g, err := h.svc.CreateGroupInCommunity(r.Context(), userID(r), chi.URLParam(r, "communityID"), req)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, g)
	default:
		writeActionResult(w, r, h, err)
	}
}

func (h *Handler) addExistingGroup(w http.ResponseWriter, r *http.Request) {
	err := h.svc.AddExistingGroup(r.Context(), userID(r), chi.URLParam(r, "communityID"), chi.URLParam(r, "conversationID"))
	switch {
	case errors.Is(err, ErrGroupNotOwnedByUser):
		response.Error(w, http.StatusForbidden, "group_not_owned", "you must be an admin or owner of that group to add it", requestID(r))
	default:
		writeActionResult(w, r, h, err)
	}
}

func (h *Handler) removeGroup(w http.ResponseWriter, r *http.Request) {
	err := h.svc.RemoveGroup(r.Context(), userID(r), chi.URLParam(r, "communityID"), chi.URLParam(r, "conversationID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) generateInviteLink(w http.ResponseWriter, r *http.Request) {
	code, err := h.svc.GenerateInviteLink(r.Context(), userID(r), chi.URLParam(r, "communityID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, InviteLinkResponse{InviteCode: code})
	default:
		writeActionResult(w, r, h, err)
	}
}

func (h *Handler) revokeInviteLink(w http.ResponseWriter, r *http.Request) {
	err := h.svc.RevokeInviteLink(r.Context(), userID(r), chi.URLParam(r, "communityID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) mine(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Mine(r.Context(), userID(r))
	if err != nil {
		h.log.Error("list my communities failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list your communities", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, res)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		h.log.Error("community search failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "search failed", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, res)
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	var req ReportCommunityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Reason == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "reason is required", requestID(r))
		return
	}
	if err := h.svc.Report(r.Context(), userID(r), chi.URLParam(r, "communityID"), req.Reason); err != nil {
		h.log.Error("report community failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to submit report", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "reported"})
}

func writeActionResult(w http.ResponseWriter, r *http.Request, h *Handler, err error) {
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "not found", requestID(r))
	case errors.Is(err, ErrNotAMember):
		response.Error(w, http.StatusForbidden, "not_a_member", "you are not a member of this community", requestID(r))
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "permission_denied", "you don't have permission to do that", requestID(r))
	case errors.Is(err, ErrCannotBanOwner):
		response.Error(w, http.StatusForbidden, "cannot_ban_owner", "the owner can't be banned or removed", requestID(r))
	case errors.Is(err, ErrEmptyName):
		response.Error(w, http.StatusBadRequest, "invalid_name", "name is required", requestID(r))
	default:
		h.log.Error("community action failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "operation failed", requestID(r))
	}
}
