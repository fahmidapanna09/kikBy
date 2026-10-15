package channels

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
	r.Post("/", h.create)
	r.Get("/search", h.search)
	r.Get("/mine", h.mine)
	r.Get("/{channelID}", h.get)
	r.Patch("/{channelID}", h.update)
	r.Delete("/{channelID}", h.delete)
	r.Post("/{channelID}/follow", h.follow)
	r.Post("/follow/{inviteCode}", h.followByInvite)
	r.Delete("/{channelID}/follow", h.unfollow)
	r.Put("/{channelID}/mute", h.setMuted)
	r.Get("/{channelID}/admins", h.listAdmins)
	r.Post("/{channelID}/admins/{userID}", h.addAdmin)
	r.Delete("/{channelID}/admins/{userID}", h.removeAdmin)
	r.Post("/{channelID}/invite-link", h.generateInviteLink)
	r.Delete("/{channelID}/invite-link", h.revokeInviteLink)
	r.Post("/{channelID}/report", h.report)
	r.Post("/{channelID}/posts", h.createPost)
	r.Get("/{channelID}/posts", h.listPosts)
	r.Patch("/posts/{postID}", h.editPost)
	r.Delete("/posts/{postID}", h.deletePost)
	r.Post("/posts/{postID}/view", h.viewPost)
	r.Post("/posts/{postID}/reactions", h.react)
	r.Delete("/posts/{postID}/reactions", h.unreact)
	r.Post("/posts/{postID}/vote", h.vote)
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
	var req CreateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	c, err := h.svc.CreateChannel(r.Context(), userID(r), req)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, c)
	case errors.Is(err, ErrEmptyName):
		response.Error(w, http.StatusBadRequest, "invalid_name", "name is required", requestID(r))
	default:
		h.log.Error("create channel failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to create channel", requestID(r))
	}
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), userID(r), chi.URLParam(r, "channelID"))
	writeChannelResult(w, r, h, c, err)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	c, err := h.svc.Update(r.Context(), userID(r), chi.URLParam(r, "channelID"), req)
	writeChannelResult(w, r, h, c, err)
}

func writeChannelResult(w http.ResponseWriter, r *http.Request, h *Handler, c ChannelSummary, err error) {
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, c)
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "channel not found", requestID(r))
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "permission_denied", "only an admin of this channel can do that", requestID(r))
	case errors.Is(err, ErrEmptyName):
		response.Error(w, http.StatusBadRequest, "invalid_name", "name is required", requestID(r))
	default:
		h.log.Error("channel operation failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "operation failed", requestID(r))
	}
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Delete(r.Context(), userID(r), chi.URLParam(r, "channelID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) follow(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Follow(r.Context(), userID(r), chi.URLParam(r, "channelID"))
	writeFollowResult(w, r, h, res, err)
}

func (h *Handler) followByInvite(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.FollowByInvite(r.Context(), userID(r), chi.URLParam(r, "inviteCode"))
	switch {
	case errors.Is(err, ErrInviteNotFound):
		response.Error(w, http.StatusNotFound, "invite_not_found", "this invite link is invalid or has been revoked", requestID(r))
	default:
		writeFollowResult(w, r, h, res, err)
	}
}

func writeFollowResult(w http.ResponseWriter, r *http.Request, h *Handler, res FollowResponse, err error) {
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "channel not found", requestID(r))
	default:
		h.log.Error("follow channel failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to follow channel", requestID(r))
	}
}

func (h *Handler) unfollow(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Unfollow(r.Context(), userID(r), chi.URLParam(r, "channelID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) setMuted(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Muted bool `json:"muted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	err := h.svc.SetMuted(r.Context(), userID(r), chi.URLParam(r, "channelID"), req.Muted)
	writeActionResult(w, r, h, err)
}

func (h *Handler) mine(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Mine(r.Context(), userID(r))
	if err != nil {
		h.log.Error("list my channels failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list your channels", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, SearchResponse{Channels: list})
}

func (h *Handler) listAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := h.svc.ListAdmins(r.Context(), userID(r), chi.URLParam(r, "channelID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]any{"admins": admins})
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "permission_denied", "only an admin can see the admin list", requestID(r))
	default:
		h.log.Error("list admins failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list admins", requestID(r))
	}
}

func (h *Handler) addAdmin(w http.ResponseWriter, r *http.Request) {
	err := h.svc.AddAdmin(r.Context(), userID(r), chi.URLParam(r, "channelID"), chi.URLParam(r, "userID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) removeAdmin(w http.ResponseWriter, r *http.Request) {
	err := h.svc.RemoveAdmin(r.Context(), userID(r), chi.URLParam(r, "channelID"), chi.URLParam(r, "userID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) generateInviteLink(w http.ResponseWriter, r *http.Request) {
	code, err := h.svc.GenerateInviteLink(r.Context(), userID(r), chi.URLParam(r, "channelID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, InviteLinkResponse{InviteCode: code})
	default:
		writeActionResult(w, r, h, err)
	}
}

func (h *Handler) revokeInviteLink(w http.ResponseWriter, r *http.Request) {
	err := h.svc.RevokeInviteLink(r.Context(), userID(r), chi.URLParam(r, "channelID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Search(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("category"))
	if err != nil {
		h.log.Error("channel search failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "search failed", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, res)
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	var req ReportChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Reason == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "reason is required", requestID(r))
		return
	}
	if err := h.svc.Report(r.Context(), userID(r), chi.URLParam(r, "channelID"), req.Reason); err != nil {
		h.log.Error("report channel failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to submit report", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "reported"})
}

// --- Posts ---

func (h *Handler) createPost(w http.ResponseWriter, r *http.Request) {
	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Type == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "type is required", requestID(r))
		return
	}
	post, err := h.svc.CreatePost(r.Context(), userID(r), chi.URLParam(r, "channelID"), req)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, post)
	case errors.Is(err, ErrInvalidPost):
		response.Error(w, http.StatusBadRequest, "invalid_post", err.Error(), requestID(r))
	case errors.Is(err, ErrInvalidMedia):
		response.Error(w, http.StatusBadRequest, "invalid_media", err.Error(), requestID(r))
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "permission_denied", "only an admin of this channel can post", requestID(r))
	case errors.Is(err, ErrRateLimited):
		response.Error(w, http.StatusTooManyRequests, "rate_limited", "too many posts recently, try again later", requestID(r))
	default:
		h.log.Error("create channel post failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to post", requestID(r))
	}
}

func (h *Handler) listPosts(w http.ResponseWriter, r *http.Request) {
	var beforeSeq int64
	if c := r.URL.Query().Get("cursor"); c != "" {
		n, err := strconv.ParseInt(c, 10, 64)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid_cursor", "cursor must be an integer seq", requestID(r))
			return
		}
		beforeSeq = n
	}
	res, err := h.svc.ListPosts(r.Context(), userID(r), chi.URLParam(r, "channelID"), beforeSeq)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "channel not found", requestID(r))
	default:
		h.log.Error("list channel posts failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list posts", requestID(r))
	}
}

func (h *Handler) editPost(w http.ResponseWriter, r *http.Request) {
	var req EditPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Body == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "body is required", requestID(r))
		return
	}
	post, err := h.svc.EditPost(r.Context(), userID(r), chi.URLParam(r, "postID"), req.Body)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, post)
	case errors.Is(err, ErrInvalidPost):
		response.Error(w, http.StatusBadRequest, "invalid_post", err.Error(), requestID(r))
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "permission_denied", "only the author or channel owner can edit this post", requestID(r))
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "post not found", requestID(r))
	default:
		h.log.Error("edit channel post failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to edit post", requestID(r))
	}
}

func (h *Handler) deletePost(w http.ResponseWriter, r *http.Request) {
	err := h.svc.DeletePost(r.Context(), userID(r), chi.URLParam(r, "postID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) viewPost(w http.ResponseWriter, r *http.Request) {
	err := h.svc.RecordView(r.Context(), userID(r), chi.URLParam(r, "postID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) react(w http.ResponseWriter, r *http.Request) {
	var req ReactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Emoji == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "emoji is required", requestID(r))
		return
	}
	err := h.svc.React(r.Context(), userID(r), chi.URLParam(r, "postID"), req.Emoji)
	writeActionResult(w, r, h, err)
}

func (h *Handler) unreact(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Unreact(r.Context(), userID(r), chi.URLParam(r, "postID"))
	writeActionResult(w, r, h, err)
}

func (h *Handler) vote(w http.ResponseWriter, r *http.Request) {
	var req VoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OptionID == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "option_id is required", requestID(r))
		return
	}
	err := h.svc.Vote(r.Context(), userID(r), chi.URLParam(r, "postID"), req.OptionID)
	writeActionResult(w, r, h, err)
}

func writeActionResult(w http.ResponseWriter, r *http.Request, h *Handler, err error) {
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "not found", requestID(r))
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "permission_denied", "you don't have permission to do that", requestID(r))
	case errors.Is(err, ErrCannotDemoteOwner):
		response.Error(w, http.StatusForbidden, "cannot_demote_owner", "the owner can't be removed as admin", requestID(r))
	case errors.Is(err, ErrInvalidPost):
		response.Error(w, http.StatusBadRequest, "invalid_input", err.Error(), requestID(r))
	default:
		h.log.Error("channel action failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "operation failed", requestID(r))
	}
}
