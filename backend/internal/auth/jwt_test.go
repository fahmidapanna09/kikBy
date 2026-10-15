package auth

import (
	"testing"
	"time"
)

func TestIssueAndVerifyAccessToken(t *testing.T) {
	svc := NewJWTService("access-secret-for-tests", "refresh-secret-for-tests", 15*time.Minute, 30*24*time.Hour)

	token, exp, err := svc.IssueAccessToken("user-123", "device-abc")
	if err != nil {
		t.Fatalf("IssueAccessToken returned error: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if !exp.After(time.Now()) {
		t.Fatal("expected expiry in the future")
	}

	userID, deviceID, err := svc.VerifyAccessToken(token)
	if err != nil {
		t.Fatalf("VerifyAccessToken returned error: %v", err)
	}
	if userID != "user-123" || deviceID != "device-abc" {
		t.Fatalf("got userID=%s deviceID=%s, want user-123/device-abc", userID, deviceID)
	}
}

func TestVerifyAccessToken_WrongSecretRejected(t *testing.T) {
	issuer := NewJWTService("secret-a", "refresh-secret", 15*time.Minute, 30*24*time.Hour)
	verifier := NewJWTService("secret-b", "refresh-secret", 15*time.Minute, 30*24*time.Hour)

	token, _, err := issuer.IssueAccessToken("user-123", "device-abc")
	if err != nil {
		t.Fatalf("IssueAccessToken returned error: %v", err)
	}

	if _, _, err := verifier.VerifyAccessToken(token); err == nil {
		t.Fatal("expected verification to fail with a different secret")
	}
}

func TestVerifyAccessToken_ExpiredRejected(t *testing.T) {
	svc := NewJWTService("access-secret", "refresh-secret", -1*time.Minute, 30*24*time.Hour)

	token, _, err := svc.IssueAccessToken("user-123", "device-abc")
	if err != nil {
		t.Fatalf("IssueAccessToken returned error: %v", err)
	}

	if _, _, err := svc.VerifyAccessToken(token); err == nil {
		t.Fatal("expected verification to fail for an already-expired token")
	}
}

func TestGenerateRefreshToken_UniqueAndHashConsistent(t *testing.T) {
	raw1, hash1, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken returned error: %v", err)
	}
	raw2, hash2, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken returned error: %v", err)
	}

	if raw1 == raw2 {
		t.Fatal("expected two generated refresh tokens to differ")
	}
	if HashRefreshToken(raw1) != hash1 {
		t.Fatal("HashRefreshToken(raw1) should equal the hash returned alongside it")
	}
	if hash1 == hash2 {
		t.Fatal("expected hashes of different tokens to differ")
	}
}
