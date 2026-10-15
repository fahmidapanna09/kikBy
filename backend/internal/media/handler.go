package media

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
	r.Post("/presign-upload", h.presignUpload)
	r.Post("/{mediaID}/confirm", h.confirmUpload)
	r.Get("/{mediaID}/download-url", h.downloadURL)
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

func (h *Handler) presignUpload(w http.ResponseWriter, r *http.Request) {
	var req PresignUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Kind == "" || req.MimeType == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "kind and mime_type are required", requestID(r))
		return
	}

	res, err := h.svc.PresignUpload(r.Context(), userID(r), req)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrMimeNotAllowed):
		response.Error(w, http.StatusBadRequest, "mime_not_allowed", "this mime type is not allowed for the given kind", requestID(r))
	default:
		h.log.Error("presign upload failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to presign upload", requestID(r))
	}
}

func (h *Handler) confirmUpload(w http.ResponseWriter, r *http.Request) {
	mediaID := chi.URLParam(r, "mediaID")
	res, err := h.svc.ConfirmUpload(r.Context(), userID(r), mediaID)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, "forbidden", "you do not own this media object", requestID(r))
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "media not found", requestID(r))
	case errors.Is(err, ErrNotUploadedYet):
		response.Error(w, http.StatusConflict, "not_uploaded_yet", "the object has not finished uploading to storage", requestID(r))
	default:
		h.log.Error("confirm upload failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to confirm upload", requestID(r))
	}
}

func (h *Handler) downloadURL(w http.ResponseWriter, r *http.Request) {
	mediaID := chi.URLParam(r, "mediaID")
	res, err := h.svc.PresignDownloadVariant(r.Context(), userID(r), mediaID, r.URL.Query().Get("variant") == "thumb")
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrForbidden):
		// Same 403 whether the media doesn't exist or isn't accessible —
		// avoid leaking existence of arbitrary media IDs to non-members.
		response.Error(w, http.StatusForbidden, "forbidden", "you do not have access to this media", requestID(r))
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusForbidden, "forbidden", "you do not have access to this media", requestID(r))
	case errors.Is(err, ErrNotUploadedYet):
		response.Error(w, http.StatusConflict, "not_uploaded_yet", "this file has not finished uploading", requestID(r))
	default:
		h.log.Error("download url failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to generate download url", requestID(r))
	}
}
