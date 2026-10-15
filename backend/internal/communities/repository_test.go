package communities

import (
	"regexp"
	"testing"
)

func TestGenerateInviteCode_FormatAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	nonAlnum := regexp.MustCompile(`[^a-z0-9]`)
	for i := 0; i < 20; i++ {
		code, err := generateInviteCode()
		if err != nil {
			t.Fatalf("generateInviteCode returned error: %v", err)
		}
		if code == "" {
			t.Fatal("expected a non-empty invite code")
		}
		if nonAlnum.MatchString(code) {
			t.Fatalf("invite code %q contains a non-alphanumeric character", code)
		}
		if seen[code] {
			t.Fatalf("generated a duplicate invite code: %q", code)
		}
		seen[code] = true
	}
}
