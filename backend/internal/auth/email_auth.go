package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,24}$`)

// normalizeEmail lower-cases and trims. Returns "" if it is not a plain
// addr-spec ("Name <a@b>" and similar display-name forms are rejected).
func normalizeEmail(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || len(s) > 254 {
		return ""
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || addr.Name != "" {
		return ""
	}
	at := strings.LastIndex(s, "@")
	if at < 1 || !strings.Contains(s[at+1:], ".") {
		return ""
	}
	return strings.ToLower(s)
}

func normalizeUsername(raw string) string { return strings.ToLower(strings.TrimSpace(raw)) }

// validatePassword mirrors the web client (>= 8 chars, a letter and a digit);
// the upper bound keeps Argon2 work bounded for hostile input.
func validatePassword(pw string) string {
	if utf8.RuneCountInString(pw) < 8 {
		return "use at least 8 characters"
	}
	if len(pw) > 128 {
		return "password is too long (max 128 bytes)"
	}
	var letter, digit bool
	for _, r := range pw {
		if unicode.IsLetter(r) {
			letter = true
		}
		if unicode.IsDigit(r) {
			digit = true
		}
	}
	if !letter || !digit {
		return "include at least one letter and one number"
	}
	return ""
}

func validateDisplayName(n string) string {
	l := utf8.RuneCountInString(strings.TrimSpace(n))
	switch {
	case l < 2:
		return "name is too short"
	case l > 50:
		return "name is too long (max 50)"
	}
	return ""
}

// normalizeContactPhone accepts "+", digits, spaces, dashes and parentheses
// (what the web form allows) and returns E.164, or ok=false when invalid.
// An empty input is valid and returns "".
func normalizeContactPhone(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", true
	}
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')':
		default:
			return "", false
		}
	}
	out := b.String()
	if !strings.HasPrefix(out, "+") {
		out = "+" + out
	}
	if !isValidE164(out) {
		return "", false
	}
	return out, true
}

// --- password hashing (Argon2id, self-describing encoded form) ---

const pwSaltLen = 16

// hashPassword returns "argon2id$t$mKiB$p$b64salt$b64hash". Parameters are
// stored with the hash so they can be raised later without invalidating
// existing accounts.
func hashPassword(pw string) (string, error) {
	salt := make([]byte, pwSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, argon2Time, argon2MemKiB, argon2Threads, argon2KeyLen)
	return fmt.Sprintf("argon2id$%d$%d$%d$%s$%s", argon2Time, argon2MemKiB, argon2Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// verifyPassword never errors: any malformed stored value is simply "no match".
func verifyPassword(stored, pw string) bool {
	p := strings.Split(stored, "$")
	if len(p) != 6 || p[0] != "argon2id" {
		return false
	}
	var t, m, th uint32
	if _, err := fmt.Sscanf(p[1]+" "+p[2]+" "+p[3], "%d %d %d", &t, &m, &th); err != nil {
		return false
	}
	// Refuse absurd parameters from a corrupted row rather than exhausting memory.
	if t < 1 || t > 10 || m < 8 || m > 256*1024 || th < 1 || th > 8 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(p[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(p[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, uint8(th), uint32(len(want)))
	return subtle.ConstantTimeCompare(want, got) == 1
}
