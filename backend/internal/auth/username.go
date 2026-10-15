package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// generateUsername produces a random, always-valid-format username for a
// new account ("user_" + 10 hex chars). Collisions are handled by the
// caller retrying on a unique-constraint violation (see repository.go) —
// this function doesn't check the database itself.
func generateUsername() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "user_" + hex.EncodeToString(b), nil
}

// hashPhoneNumber produces the value stored in users.phone_number_hash,
// used ONLY for contact-sync matching (see internal/contacts) — it lets the
// server match a client-side phone-number hash against registered accounts
// without either side ever transmitting other people's raw phone numbers
// unnecessarily. Must stay byte-for-byte consistent with how the migration
// backfilled existing rows (encode(digest(phone_number,'sha256'),'hex') in
// Postgres) and with whatever the Android client computes client-side for
// contact sync — plain lowercase-hex SHA-256, no salt. A per-install salt
// would be more resistant to precomputed rainbow-table matching, but would
// also mean the server (and every client) needs to agree on and store that
// salt just to do a lookup that's already scoped to "does this exact E.164
// number belong to a registered account" — same information the phone
// number search endpoint already exposes one-at-a-time, rate-limited. Not
// adding a salt here is a deliberate scope decision, not an oversight.
func hashPhoneNumber(phoneNumber string) string {
	sum := sha256.Sum256([]byte(phoneNumber))
	return hex.EncodeToString(sum[:])
}
