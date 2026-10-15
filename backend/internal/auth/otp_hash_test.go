package auth

import "testing"

func TestHashOTPCode_RoundTrips(t *testing.T) {
	stored, err := hashOTPCode("482913")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !verifyOTPCode(stored, "482913") {
		t.Fatalf("expected the correct code to verify")
	}
}

func TestHashOTPCode_WrongCodeFails(t *testing.T) {
	stored, err := hashOTPCode("482913")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verifyOTPCode(stored, "000000") {
		t.Fatalf("expected a wrong code to fail verification")
	}
}

func TestHashOTPCode_NeverStoresPlaintext(t *testing.T) {
	stored, err := hashOTPCode("482913")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored == "482913" {
		t.Fatalf("stored value must never equal the plaintext code")
	}
	// Loosely confirm the stored value looks like salt:hash, not a bare code.
	if len(stored) < 20 {
		t.Fatalf("stored value looks too short to be salt:hash: %q", stored)
	}
}

func TestVerifyOTPCode_MalformedStoredValueFailsClosed(t *testing.T) {
	if verifyOTPCode("not-a-valid-stored-value", "482913") {
		t.Fatalf("a malformed stored value must never verify successfully")
	}
	if verifyOTPCode("", "482913") {
		t.Fatalf("an empty stored value must never verify successfully")
	}
}

func TestHashOTPCode_DifferentSaltsPerCall(t *testing.T) {
	a, err := hashOTPCode("482913")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := hashOTPCode("482913")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == b {
		t.Fatalf("expected different salts to produce different stored values for the same code")
	}
	// Both must still independently verify against the same candidate code.
	if !verifyOTPCode(a, "482913") || !verifyOTPCode(b, "482913") {
		t.Fatalf("both independently-salted hashes must verify the same code")
	}
}
