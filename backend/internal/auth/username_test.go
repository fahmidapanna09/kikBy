package auth

import (
	"regexp"
	"testing"
)

func TestGenerateUsername_FormatAndUniqueness(t *testing.T) {
	pattern := regexp.MustCompile(`^user_[0-9a-f]{10}$`)
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		u, err := generateUsername()
		if err != nil {
			t.Fatalf("generateUsername returned error: %v", err)
		}
		if !pattern.MatchString(u) {
			t.Fatalf("generateUsername() = %q, does not match expected format", u)
		}
		if seen[u] {
			t.Fatalf("generateUsername produced a duplicate: %q", u)
		}
		seen[u] = true
	}
}

func TestHashPhoneNumber_DeterministicAndDistinct(t *testing.T) {
	h1 := hashPhoneNumber("+8801712345678")
	h2 := hashPhoneNumber("+8801712345678")
	h3 := hashPhoneNumber("+8801712345679")

	if h1 != h2 {
		t.Fatal("hashPhoneNumber should be deterministic for the same input")
	}
	if h1 == h3 {
		t.Fatal("hashPhoneNumber should differ for different inputs")
	}
	if len(h1) != 64 { // hex-encoded SHA-256 = 32 bytes = 64 hex chars
		t.Fatalf("expected a 64-character hex digest, got length %d", len(h1))
	}
}
