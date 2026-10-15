package contacts

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
	r.Post("/sync", h.sync)
	r.Post("/", h.addContact)
	r.Get("/", h.listContacts)
	r.Patch("/{contactID}", h.updateContact)
	r.Delete("/{contactID}", h.deleteContact)
	r.Post("/block/{userID}", h.blockUser)
	r.Delete("/block/{userID}", h.unblockUser)
	r.Get("/blocked", h.listBlocked)
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

func (h *Handler) addContact(w http.ResponseWriter, r *http.Request) {
	var req AddContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PhoneNumber == "" {
		response.Error(w, http.StatusBadRequest, "invalid_body", "phone_number is required", requestID(r))
		return
	}
	contact, err := h.svc.AddContact(r.Context(), userID(r), req)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, contact)
	case errors.Is(err, ErrInvalidPhoneNumber):
		response.Error(w, http.StatusBadRequest, "invalid_phone_number", err.Error(), requestID(r))
	default:
		h.log.Error("add contact failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to add contact", requestID(r))
	}
}

func (h *Handler) listContacts(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListContacts(r.Context(), userID(r))
	if err != nil {
		h.log.Error("list contacts failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list contacts", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, res)
}

func (h *Handler) updateContact(w http.ResponseWriter, r *http.Request) {
	contactID := chi.URLParam(r, "contactID")
	var req UpdateContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	err := h.svc.UpdateContact(r.Context(), userID(r), contactID, req)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "contact not found", requestID(r))
	default:
		h.log.Error("update contact failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to update contact", requestID(r))
	}
}

func (h *Handler) deleteContact(w http.ResponseWriter, r *http.Request) {
	contactID := chi.URLParam(r, "contactID")
	err := h.svc.DeleteContact(r.Context(), userID(r), contactID)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "contact not found", requestID(r))
	default:
		h.log.Error("delete contact failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to delete contact", requestID(r))
	}
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	var req SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	res, err := h.svc.Sync(r.Context(), req)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrTooManyHashes):
		response.Error(w, http.StatusBadRequest, "too_many_hashes", err.Error(), requestID(r))
	default:
		h.log.Error("contact sync failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "sync failed", requestID(r))
	}
}

func (h *Handler) blockUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	err := h.svc.BlockUser(r.Context(), userID(r), targetID)
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "blocked"})
	case errors.Is(err, ErrCannotBlockSelf):
		response.Error(w, http.StatusBadRequest, "cannot_block_self", err.Error(), requestID(r))
	default:
		h.log.Error("block user failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to block user", requestID(r))
	}
}

func (h *Handler) unblockUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	if err := h.svc.UnblockUser(r.Context(), userID(r), targetID); err != nil {
		h.log.Error("unblock user failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to unblock user", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "unblocked"})
}

func (h *Handler) listBlocked(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListBlocked(r.Context(), userID(r))
	if err != nil {
		h.log.Error("list blocked failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list blocked users", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, res)
}
