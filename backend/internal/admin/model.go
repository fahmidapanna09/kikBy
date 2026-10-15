// Package admin implements the RBAC admin panel (Phase 11): user search,
// suspend/unsuspend, channel-report moderation, and read-only metrics.
//
// Module boundary, same rule as every other module in this project: admin
// never queries another module's tables directly for anything that isn't
// its own (admin_roles is the one table this package owns). User account
// mutation goes through the UserDirectory/UserModerator interfaces below,
// satisfied structurally by *auth.Service; report moderation goes through
// ReportsProvider, satisfied structurally by *channels.Service. Metrics
// (metrics.go) are the one deliberate, narrow exception — see that file's
// doc comment for why.
package admin

import (
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("admin: not found")
	ErrInvalidRole      = errors.New("admin: invalid role")
	ErrPermissionDenied = errors.New("admin: permission denied")
	ErrSelfRoleChange   = errors.New("admin: cannot change your own admin role")
)

type Role string

const (
	RoleModerator  Role = "moderator"
	RoleAdmin      Role = "admin"
	RoleSuperadmin Role = "superadmin"
)

// rank gives each role a total order for "at least this privileged"
// checks (RequireRole). Higher is more privileged. An empty/unknown role
// (i.e. "not an admin at all") ranks below every real role.
func rank(r Role) int {
	switch r {
	case RoleModerator:
		return 1
	case RoleAdmin:
		return 2
	case RoleSuperadmin:
		return 3
	default:
		return 0
	}
}

func isValidRole(r Role) bool {
	return r == RoleModerator || r == RoleAdmin || r == RoleSuperadmin
}

type AdminRoleInfo struct {
	UserID    string    `json:"user_id"`
	Role      Role      `json:"role"`
	GrantedBy *string   `json:"granted_by,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
}

type GrantRoleRequest struct {
	Role string `json:"role"`
}

type SuspendUserRequest struct {
	Reason string `json:"reason"`
}

type ResolveReportRequest struct {
	Note string `json:"note"`
}
