package admin

import (
	"context"
	"strings"

	"github.com/yourorg/chatplatform/internal/platform/audit"
)

// UserDirectory is satisfied structurally by *auth.Service. Every method
// returns plain maps/primitives (never a *auth.User or similar), so this
// package and auth never import each other — see
// internal/auth/admin_repository.go's doc comment for the full reasoning,
// shared by every cross-module interface in this project.
type UserDirectory interface {
	GetUserIDByPhone(ctx context.Context, phoneNumber string) (string, error)
	GetUserAdmin(ctx context.Context, userID string) (map[string]any, error)
	SearchUsersAdmin(ctx context.Context, query string, limit, offset int) ([]map[string]any, int, error)
}

// UserModerator is satisfied structurally by *auth.Service.
type UserModerator interface {
	SuspendUser(ctx context.Context, targetUserID, reason, actingAdminID string) error
	UnsuspendUser(ctx context.Context, targetUserID, actingAdminID string) error
}

// ReportsProvider is satisfied structurally by *channels.Service.
type ReportsProvider interface {
	ListReportsAdmin(ctx context.Context, onlyUnresolved bool, limit, offset int) ([]map[string]any, int, error)
	ResolveReportAdmin(ctx context.Context, reportID, resolvedByUserID, note string) error
}

type Service struct {
	repo    *Repository
	metrics *MetricsRepository
	users   UserDirectory
	mod     UserModerator
	reports ReportsProvider
	audit   *audit.Logger
}

func NewService(repo *Repository, metrics *MetricsRepository, users UserDirectory, mod UserModerator, reports ReportsProvider, auditLogger *audit.Logger) *Service {
	return &Service{repo: repo, metrics: metrics, users: users, mod: mod, reports: reports, audit: auditLogger}
}

// GetRole returns ("", false, nil) — not an error — for a user who simply
// isn't an admin at all; that's the overwhelmingly common case and callers
// (RequireRole below) treat it as "no access", not as a failure to look up.
func (s *Service) GetRole(ctx context.Context, userID string) (Role, bool, error) {
	role, err := s.repo.GetRole(ctx, userID)
	if err == ErrNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return role, true, nil
}

// GrantRole requires the acting user to already be a superadmin — the one
// role allowed to manage other admins. A superadmin cannot change their
// own role (ErrSelfRoleChange) so the system can't be left admin-less by
// one person locking themselves out, and granting always needs an
// existing real user account (looked up via UserDirectory, never trusted
// as a bare ID from the request).
func (s *Service) GrantRole(ctx context.Context, actingAdminID, targetUserID string, role Role) error {
	if !isValidRole(role) {
		return ErrInvalidRole
	}
	if targetUserID == actingAdminID {
		return ErrSelfRoleChange
	}
	actingRole, isAdmin, err := s.GetRole(ctx, actingAdminID)
	if err != nil {
		return err
	}
	if !isAdmin || actingRole != RoleSuperadmin {
		return ErrPermissionDenied
	}
	if _, err := s.users.GetUserAdmin(ctx, targetUserID); err != nil {
		return err
	}
	if err := s.repo.SetRole(ctx, targetUserID, role, actingAdminID); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.EventAdminRoleGranted, actingAdminID, "", "", map[string]any{
		"target_user_id": targetUserID, "role": string(role),
	})
	return nil
}

// RevokeRole refuses to remove the last superadmin — without at least one,
// nobody could ever grant another admin role again.
func (s *Service) RevokeRole(ctx context.Context, actingAdminID, targetUserID string) error {
	actingRole, isAdmin, err := s.GetRole(ctx, actingAdminID)
	if err != nil {
		return err
	}
	if !isAdmin || actingRole != RoleSuperadmin {
		return ErrPermissionDenied
	}
	if targetUserID == actingAdminID {
		return ErrSelfRoleChange
	}

	targetRole, err := s.repo.GetRole(ctx, targetUserID)
	if err != nil {
		return err
	}
	if targetRole == RoleSuperadmin {
		count, err := s.repo.CountByRole(ctx, RoleSuperadmin)
		if err != nil {
			return err
		}
		if count <= 1 {
			return ErrPermissionDenied
		}
	}

	if err := s.repo.RemoveRole(ctx, targetUserID); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.EventAdminRoleRevoked, actingAdminID, "", "", map[string]any{
		"target_user_id": targetUserID,
	})
	return nil
}

func (s *Service) ListAdmins(ctx context.Context) ([]AdminRoleInfo, error) {
	return s.repo.ListAdmins(ctx)
}

// BootstrapSuperadmins is called once at startup (see cmd/api/main.go)
// with phone numbers from ADMIN_BOOTSTRAP_PHONE_NUMBERS. It's the only way
// the very first superadmin gets created — every other grant requires an
// existing superadmin to make it, which is impossible before one exists.
// Idempotent: re-running it (every restart) on an already-granted phone
// number just re-confirms the role, it doesn't error or duplicate.
func (s *Service) BootstrapSuperadmins(ctx context.Context, phoneNumbers []string) error {
	for _, phone := range phoneNumbers {
		phone = strings.TrimSpace(phone)
		if phone == "" {
			continue
		}
		userID, err := s.users.GetUserIDByPhone(ctx, phone)
		if err != nil {
			// Not registered yet — not fatal to startup; they'll need to
			// sign up first, then a restart (or a future re-bootstrap
			// endpoint, not built here) picks them up.
			continue
		}
		if err := s.repo.SetRole(ctx, userID, RoleSuperadmin, userID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SearchUsers(ctx context.Context, query string, limit, offset int) ([]map[string]any, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	return s.users.SearchUsersAdmin(ctx, query, limit, offset)
}

func (s *Service) GetUser(ctx context.Context, userID string) (map[string]any, error) {
	return s.users.GetUserAdmin(ctx, userID)
}

func (s *Service) SuspendUser(ctx context.Context, actingAdminID, targetUserID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = "no reason given"
	}
	return s.mod.SuspendUser(ctx, targetUserID, reason, actingAdminID)
}

func (s *Service) UnsuspendUser(ctx context.Context, actingAdminID, targetUserID string) error {
	return s.mod.UnsuspendUser(ctx, targetUserID, actingAdminID)
}

func (s *Service) ListReports(ctx context.Context, onlyUnresolved bool, limit, offset int) ([]map[string]any, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	return s.reports.ListReportsAdmin(ctx, onlyUnresolved, limit, offset)
}

func (s *Service) ResolveReport(ctx context.Context, actingAdminID, reportID, note string) error {
	if err := s.reports.ResolveReportAdmin(ctx, reportID, actingAdminID, note); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.EventReportResolved, actingAdminID, "", "", map[string]any{"report_id": reportID})
	return nil
}

func (s *Service) MetricsOverview(ctx context.Context) (map[string]any, error) {
	return s.metrics.Overview(ctx)
}

func (s *Service) MetricsGrowth(ctx context.Context, days int) ([]map[string]any, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	return s.metrics.GrowthSeries(ctx, days)
}
