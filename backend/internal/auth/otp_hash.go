package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Phase 10 hardening: OTP codes used to be stored in Redis as plaintext.
// Redis is trusted infra (not exposed to clients), but the spec's security
// model ("Argon2id for any stored secret material") doesn't carve out an
// exception for "trusted infra", and a misconfigured log shipper, a Redis
// snapshot leaving the VPC, or a compromised Redis instance would otherwise
// hand over every currently-valid login code directly. Hashing closes that
// specific exposure.
//
// Honest limitation, documented rather than hidden: a 6-digit code is only
// 10^6 possibilities, so hashing does NOT make the code brute-force-proof
// offline — it raises the cost from "trivial" to "computable in a few CPU-
// seconds per code with Argon2id's default cost". The real protection
// against guessing was, is, and remains the per-phone/per-IP rate limiter
// in otp.go (3 attempts per 10 minutes) plus the 5-minute TTL. This change
// is defense against passive exposure of the store, not a claim that OTP
// codes are now cryptographically unguessable.
const (
	argon2Time    = 2
	argon2MemKiB  = 19 * 1024 // 19 MiB — OWASP's minimum-profile recommendation for Argon2id
	argon2Threads = 1
	argon2KeyLen  = 32
	otpSaltLen    = 16
)

// hashOTPCode returns "base64(salt):base64(hash)", suitable to store as the
// Redis value in place of the plaintext code.
func hashOTPCode(code string) (string, error) {
	salt := make([]byte, otpSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("otp: generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(code), salt, argon2Time, argon2MemKiB, argon2Threads, argon2KeyLen)
	return base64.RawStdEncoding.EncodeToString(salt) + ":" + base64.RawStdEncoding.EncodeToString(hash), nil
}

// verifyOTPCode re-derives the hash from candidateCode using the salt
// embedded in stored and compares in constant time. Returns false (never
// an error) for any malformed stored value — treated the same as "wrong
// code" by the caller, never as a server error.
func verifyOTPCode(stored, candidateCode string) bool {
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	wantHash, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	gotHash := argon2.IDKey([]byte(candidateCode), salt, argon2Time, argon2MemKiB, argon2Threads, argon2KeyLen)
	return subtle.ConstantTimeCompare(wantHash, gotHash) == 1
}
