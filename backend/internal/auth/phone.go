package auth

import "regexp"

// e164Pattern is intentionally simple validation at the service layer.
// Stricter validation (real country calling-code tables, number-length
// rules) belongs in a dedicated phone-number library (e.g. a Go port of
// libphonenumber) — swap this out before Phase 1 ships to production.
var e164Pattern = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

func isValidE164(phone string) bool {
	return e164Pattern.MatchString(phone)
}

// countryCodeFromPhone is a placeholder that should be replaced with a real
// libphonenumber-based lookup; for now it just returns "unknown" so the
// column is never left NULL. Not used for anything security-sensitive.
func countryCodeFromPhone(phone string) string {
	return "unknown"
}
