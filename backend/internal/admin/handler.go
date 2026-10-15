package admin

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

func requestID(r *http.Request) string {
	id, _ := r.Context().Value(middleware.CtxRequestID).(string)
	return id
}
func userID(r *http.Request) string {
	id, _ := r.Context().Value(middleware.CtxUserID).(string)
	return id
}

// requireRole is admin's own RBAC layer, separate from (and always applied
// after) middleware.RequireAuth — it answers "is this already-authenticated
// user an admin of at least this rank", not "who are they". It lives here
// rather than in the shared platform/middleware package because only
// admin's own routes need it; every other module's routes only ever need
// RequireAuth.
func (h *Handler) requireRole(minRole Role, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role, isAdmin, err := h.svc.GetRole(r.Context(), userID(r))
		if err != nil {
			h.log.Error("admin role lookup failed", zap.Error(err), zap.String("request_id", requestID(r)))
			response.Error(w, http.StatusInternalServerError, "internal_error", "failed to check admin role", requestID(r))
			return
		}
		if !isAdmin || rank(role) < rank(minRole) {
			// Deliberately the same 403 whether the caller has no admin
			// role at all or merely an insufficient one — never reveals
			// which, same reasoning as auth's "session not found" 404.
			response.Error(w, http.StatusForbidden, "admin_permission_denied", "this action requires a higher admin role", requestID(r))
			return
		}
		next(w, r)
	}
}

func (h *Handler) Routes(verifier middleware.TokenVerifier) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(verifier))

	r.Get("/me", h.me) // any authenticated user can check their own admin status — not role-gated

	r.Get("/users", h.requireRole(RoleModerator, h.searchUsers))
	r.Get("/users/{targetUserID}", h.requireRole(RoleModerator, h.getUser))
	r.Post("/users/{targetUserID}/suspend", h.requireRole(RoleModerator, h.suspendUser))
	r.Post("/users/{targetUserID}/unsuspend", h.requireRole(RoleModerator, h.unsuspendUser))

	r.Get("/reports", h.requireRole(RoleModerator, h.listReports))
	r.Post("/reports/{reportID}/resolve", h.requireRole(RoleModerator, h.resolveReport))

	r.Get("/admins", h.requireRole(RoleAdmin, h.listAdmins))
	r.Post("/admins/{targetUserID}", h.requireRole(RoleSuperadmin, h.grantRole))
	r.Delete("/admins/{targetUserID}", h.requireRole(RoleSuperadmin, h.revokeRole))

	r.Get("/metrics/overview", h.requireRole(RoleModerator, h.metricsOverview))
	r.Get("/metrics/growth", h.requireRole(RoleModerator, h.metricsGrowth))

	return r
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	role, isAdmin, err := h.svc.GetRole(r.Context(), userID(r))
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to check admin role", requestID(r))
		return
	}
	if !isAdmin {
		response.JSON(w, http.StatusOK, map[string]any{"is_admin": false})
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"is_admin": true, "role": role})
}

func paginationParams(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	return
}

func (h *Handler) searchUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset := paginationParams(r)
	users, total, err := h.svc.SearchUsers(r.Context(), r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		h.log.Error("admin search users failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to search users", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"users": users, "total": total})
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.GetUser(r.Context(), chi.URLParam(r, "targetUserID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, u)
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "user not found", requestID(r))
	default:
		h.log.Error("admin get user failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to look up user", requestID(r))
	}
}

func (h *Handler) suspendUser(w http.ResponseWriter, r *http.Request) {
	var req SuspendUserRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // reason is optional (defaults applied in Service.SuspendUser)
	err := h.svc.SuspendUser(r.Context(), userID(r), chi.URLParam(r, "targetUserID"), req.Reason)
	if err != nil {
		h.log.Error("admin suspend user failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to suspend user", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "suspended"})
}

func (h *Handler) unsuspendUser(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.UnsuspendUser(r.Context(), userID(r), chi.URLParam(r, "targetUserID")); err != nil {
		h.log.Error("admin unsuspend user failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to unsuspend user", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "active"})
}

func (h *Handler) listReports(w http.ResponseWriter, r *http.Request) {
	limit, offset := paginationParams(r)
	onlyUnresolved := r.URL.Query().Get("resolved") != "true"
	reports, total, err := h.svc.ListReports(r.Context(), onlyUnresolved, limit, offset)
	if err != nil {
		h.log.Error("admin list reports failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list reports", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"reports": reports, "total": total})
}

func (h *Handler) resolveReport(w http.ResponseWriter, r *http.Request) {
	var req ResolveReportRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := h.svc.ResolveReport(r.Context(), userID(r), chi.URLParam(r, "reportID"), req.Note); err != nil {
		h.log.Error("admin resolve report failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to resolve report", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

func (h *Handler) listAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := h.svc.ListAdmins(r.Context())
	if err != nil {
		h.log.Error("admin list admins failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to list admins", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"admins": admins})
}

func (h *Handler) grantRole(w http.ResponseWriter, r *http.Request) {
	var req GrantRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", "malformed request body", requestID(r))
		return
	}
	err := h.svc.GrantRole(r.Context(), userID(r), chi.URLParam(r, "targetUserID"), Role(req.Role))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "granted"})
	case errors.Is(err, ErrInvalidRole):
		response.Error(w, http.StatusBadRequest, "invalid_role", "role must be moderator, admin, or superadmin", requestID(r))
	case errors.Is(err, ErrSelfRoleChange):
		response.Error(w, http.StatusBadRequest, "self_role_change", "you cannot change your own admin role", requestID(r))
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "admin_permission_denied", "only a superadmin can grant admin roles", requestID(r))
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "target user not found", requestID(r))
	default:
		h.log.Error("admin grant role failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to grant role", requestID(r))
	}
}

func (h *Handler) revokeRole(w http.ResponseWriter, r *http.Request) {
	err := h.svc.RevokeRole(r.Context(), userID(r), chi.URLParam(r, "targetUserID"))
	switch {
	case err == nil:
		response.JSON(w, http.StatusOK, map[string]string{"status": "revoked"})
	case errors.Is(err, ErrSelfRoleChange):
		response.Error(w, http.StatusBadRequest, "self_role_change", "you cannot change your own admin role", requestID(r))
	case errors.Is(err, ErrPermissionDenied):
		response.Error(w, http.StatusForbidden, "admin_permission_denied", "only a superadmin can revoke admin roles, and the last superadmin can't be removed", requestID(r))
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not_found", "that user has no admin role", requestID(r))
	default:
		h.log.Error("admin revoke role failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to revoke role", requestID(r))
	}
}

func (h *Handler) metricsOverview(w http.ResponseWriter, r *http.Request) {
	m, err := h.svc.MetricsOverview(r.Context())
	if err != nil {
		h.log.Error("admin metrics overview failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to compute metrics", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, m)
}

func (h *Handler) metricsGrowth(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	series, err := h.svc.MetricsGrowth(r.Context(), days)
	if err != nil {
		h.log.Error("admin metrics growth failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to compute growth metrics", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"series": series})
}
