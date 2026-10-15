package contacts

import "time"

type Contact struct {
	ID            string
	OwnerID       string
	ContactUserID *string // NULL until/unless the phone number matches a registered account
	PhoneNumber   string
	Nickname      *string
	AddedVia      string // manual | sync
	CreatedAt     time.Time
}

// --- Request/response DTOs ---

type AddContactRequest struct {
	PhoneNumber string `json:"phone_number"` // E.164
	Nickname    string `json:"nickname,omitempty"`
}

type UpdateContactRequest struct {
	Nickname *string `json:"nickname,omitempty"`
}

type ContactResponse struct {
	ID            string  `json:"id"`
	ContactUserID *string `json:"contact_user_id,omitempty"`
	PhoneNumber   string  `json:"phone_number"`
	Nickname      *string `json:"nickname,omitempty"`
	IsRegistered  bool    `json:"is_registered"`
	// Populated only when IsRegistered — the registered account's current
	// username/display name, so the client doesn't need a second call per
	// contact just to show a name instead of a raw phone number.
	Username    *string `json:"username,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
}

type ListContactsResponse struct {
	Contacts []ContactResponse `json:"contacts"`
}

// SyncRequest carries phone-number hashes computed CLIENT-SIDE (SHA-256 hex
// of each E.164 number in the device's address book) — the server never
// receives the plaintext number of a contact who isn't already the
// requesting user's own account or an existing manually-added contact.
type SyncRequest struct {
	PhoneHashes []string `json:"phone_hashes"`
}

type SyncMatch struct {
	PhoneHash   string  `json:"phone_hash"`
	UserID      string  `json:"user_id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name,omitempty"`
}

type SyncResponse struct {
	Matches []SyncMatch `json:"matches"`
}

type BlockedUserResponse struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName *string   `json:"display_name,omitempty"`
	BlockedAt   time.Time `json:"blocked_at"`
}

type ListBlockedResponse struct {
	Blocked []BlockedUserResponse `json:"blocked"`
}
