package admin

import "testing"

func TestRoleRankOrdering(t *testing.T) {
	if !(rank(RoleModerator) < rank(RoleAdmin) && rank(RoleAdmin) < rank(RoleSuperadmin)) {
		t.Fatalf("expected moderator < admin < superadmin, got %d, %d, %d",
			rank(RoleModerator), rank(RoleAdmin), rank(RoleSuperadmin))
	}
}

func TestRankOfNonAdminIsLowestOfAll(t *testing.T) {
	if rank("") >= rank(RoleModerator) {
		t.Fatalf("a non-admin (empty role) must rank below moderator, the lowest real role")
	}
	if rank("garbage") >= rank(RoleModerator) {
		t.Fatalf("an unrecognized role string must rank below moderator, not be treated as privileged")
	}
}

func TestIsValidRole(t *testing.T) {
	for _, r := range []Role{RoleModerator, RoleAdmin, RoleSuperadmin} {
		if !isValidRole(r) {
			t.Fatalf("expected %q to be a valid role", r)
		}
	}
	for _, r := range []Role{"", "owner", "Admin", "SUPERADMIN"} {
		if isValidRole(Role(r)) {
			t.Fatalf("expected %q to be rejected as an invalid role (case-sensitive, exact match only)", r)
		}
	}
}

func TestDistinctSentinelErrors(t *testing.T) {
	errs := []error{ErrNotFound, ErrInvalidRole, ErrPermissionDenied, ErrSelfRoleChange}
	for i := range errs {
		for j := range errs {
			if i != j && errs[i] == errs[j] {
				t.Fatalf("sentinel errors at index %d and %d must be distinct", i, j)
			}
		}
	}
}
