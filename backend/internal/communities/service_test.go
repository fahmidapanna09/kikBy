package communities

import "testing"

func TestIsAdminOrOwner(t *testing.T) {
	cases := []struct {
		role Role
		want bool
	}{
		{RoleOwner, true},
		{RoleAdmin, true},
		{RoleMember, false},
	}
	for _, c := range cases {
		if got := isAdminOrOwner(c.role); got != c.want {
			t.Errorf("isAdminOrOwner(%q) = %v, want %v", c.role, got, c.want)
		}
	}
}

func TestIncludeInvite(t *testing.T) {
	if !includeInvite(RoleOwner) {
		t.Error("owner should see the invite code")
	}
	if !includeInvite(RoleAdmin) {
		t.Error("admin should see the invite code")
	}
	if includeInvite(RoleMember) {
		t.Error("a plain member should not see the invite code")
	}
}
