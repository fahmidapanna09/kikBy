package stories

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

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
	r.Get("/feed", h.feed)
	r.Get("/mine", h.mine)
	r.Get("/archive", h.archive)
	r.Delete("/{storyID}", h.delete)
	r.Post("/{storyID}/view", h.view)
	r.Get("/{storyID}/viewers", h.viewers)
	r.Post("/{storyID}/reactions", h.react)
	r.Delete("/{storyID}/reactions", h.unreact)
	r.Post("/{storyID}/replies", h.reply)
	r.Get("/{storyID}/replies", h.replies)
	r.Post("/mute/{userID}", h.mute)
	r.Delete("/mute/{userID}", h.unmute)
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
	var req CreateStoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	story, err := h.svc.CreateStory(r.Context(), userID(r), req)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, story)
	case errors.Is(err, ErrInvalidStory), errors.Is(err, ErrInvalidMention):
		response.Error(w, http.StatusBadRequest, "invalid_story", err.Error(), requestID(r))
	case errors.Is(err, ErrInvalidMedia):
		response.Error(w, http.StatusBadRequest, "invalid_media", err.Error(), requestID(r))
	case errors.Is(err, ErrRateLimited):
		response.Error(w, http.StatusTooManyRequests, "rate_limited", "too many stories posted recently, try again later", requestID(r))
	default:
		h.log.Error("create story failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to post story", requestID(r))
	}
}

func (h *Handler) feed(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Feed(r.Context(), userID(r))
	if err != nil {
		h.log.Error("story feed failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to load stories", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, res)
}

func (h *Handler) mine(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Mine(r.Context(), userID(r))
	if err != nil {
		h.log.Error("my stories failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to load your stories", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, res)
}

func (h *Handler) archive(w http.ResponseWriter, r *http.Request) {
	var before *time.Time
	if v := r.URL.Query().Get("before"); v != "" {
		ms, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid_before", "before must be a unix millisecond timestamp", requestID(r))
			return
		}
		t := time.UnixMilli(ms)
		before = &t
	}
	res, err := h.svc.Archive(r.Context(), userID(r), before)
	if err != nil {
		h.log.Error("story archive failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to load your archive", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, res)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Delete(r.Context(), userID(r), chi.URLParam(r, "storyID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "story not found", requestID(r))
	default:
		h.log.Error("delete story failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to delete story", requestID(r))
	}
}

func (h *Handler) view(w http.ResponseWriter, r *http.Request) {
	err := h.svc.RecordView(r.Context(), userID(r), chi.URLParam(r, "storyID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "story not found", requestID(r))
	default:
		h.log.Error("record view failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to record view", requestID(r))
	}
}

func (h *Handler) viewers(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Viewers(r.Context(), userID(r), chi.URLParam(r, "storyID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "story not found", requestID(r))
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, "forbidden", "only the author can see who viewed this story", requestID(r))
	default:
		h.log.Error("list viewers failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list viewers", requestID(r))
	}
}

func (h *Handler) react(w http.ResponseWriter, r *http.Request) {
	var req ReactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Emoji == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "emoji is required", requestID(r))
		return
	}
	err := h.svc.React(r.Context(), userID(r), chi.URLParam(r, "storyID"), req.Emoji)
	writeStoryActionResult(w, r, h, err)
}

func (h *Handler) unreact(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Unreact(r.Context(), userID(r), chi.URLParam(r, "storyID"))
	writeStoryActionResult(w, r, h, err)
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request) {
	var req ReplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Body == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "body is required", requestID(r))
		return
	}
	rep, err := h.svc.Reply(r.Context(), userID(r), chi.URLParam(r, "storyID"), req.Body)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, rep)
	case errors.Is(err, ErrInvalidStory):
		response.Error(w, http.StatusBadRequest, "invalid_reply", err.Error(), requestID(r))
	case errors.Is(err, ErrRateLimited):
		response.Error(w, http.StatusTooManyRequests, "rate_limited", "too many replies, slow down", requestID(r))
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "story not found", requestID(r))
	default:
		h.log.Error("story reply failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to send reply", requestID(r))
	}
}

func (h *Handler) replies(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Replies(r.Context(), userID(r), chi.URLParam(r, "storyID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "story not found", requestID(r))
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, "forbidden", "only the author can see replies to their story", requestID(r))
	default:
		h.log.Error("list replies failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list replies", requestID(r))
	}
}

func (h *Handler) mute(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Mute(r.Context(), userID(r), chi.URLParam(r, "userID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "muted"})
	case errors.Is(err, ErrCannotMuteSelf):
		response.Error(w, http.StatusBadRequest, "cannot_mute_self", err.Error(), requestID(r))
	default:
		h.log.Error("mute failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to mute", requestID(r))
	}
}

func (h *Handler) unmute(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Unmute(r.Context(), userID(r), chi.URLParam(r, "userID")); err != nil {
		h.log.Error("unmute failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to unmute", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "unmuted"})
}

func writeStoryActionResult(w http.ResponseWriter, r *http.Request, h *Handler, err error) {
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case errors.Is(err, ErrInvalidStory):
		response.Error(w, http.StatusBadRequest, "invalid_reaction", err.Error(), requestID(r))
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "story not found", requestID(r))
	default:
		h.log.Error("story action failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "operation failed", requestID(r))
	}
}
