package chat

import "testing"

func TestIsAdminOrOwner(t *testing.T) {
	cases := []struct {
		role MemberRole
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

func TestGenerateInviteCode_UniqueAndURLSafe(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		code, err := generateInviteCode()
		if err != nil {
			t.Fatalf("generateInviteCode returned error: %v", err)
		}
		if code == "" {
			t.Fatal("expected a non-empty invite code")
		}
		if seen[code] {
			t.Fatalf("generated a duplicate invite code: %q", code)
		}
		seen[code] = true
		for _, r := range code {
			if r == '/' || r == '+' || r == '=' {
				t.Fatalf("invite code %q contains a non-URL-safe character", code)
			}
		}
	}
}
