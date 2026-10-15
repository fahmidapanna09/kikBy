package users

import "time"

type Profile struct {
	UserID             string
	PhoneNumber        string // only ever returned in GET /users/me, never in public/search responses
	Username           string
	DisplayName        *string
	About              *string
	AvatarMediaID      *string
	LastSeenAt         *time.Time
	LastSeenVisibility string // everyone | contacts | nobody
}

// PublicProfile is what GET /users/{id} and search results return — never
// includes phone_number, matching "never expose private phone numbers
// through search" from the spec.
type PublicProfile struct {
	UserID        string  `json:"user_id"`
	Username      string  `json:"username"`
	DisplayName   *string `json:"display_name,omitempty"`
	About         *string `json:"about,omitempty"`
	AvatarMediaID *string `json:"avatar_media_id,omitempty"`
}

type MeResponse struct {
	UserID             string  `json:"user_id"`
	PhoneNumber        string  `json:"phone_number,omitempty"` // verified (OTP) number, if any
	Email              string  `json:"email,omitempty"`
	EmailVerified      bool    `json:"email_verified"`
	PendingEmail       string  `json:"pending_email,omitempty"` // new address awaiting confirmation
	ContactPhone       string  `json:"contact_phone,omitempty"` // optional, unverified, display only
	Username           string  `json:"username"`
	DisplayName        *string `json:"display_name,omitempty"`
	About              *string `json:"about,omitempty"`
	AvatarMediaID      *string `json:"avatar_media_id,omitempty"`
	LastSeenVisibility string  `json:"last_seen_visibility"`
}

type UpdateProfileRequest struct {
	DisplayName        *string `json:"display_name,omitempty"`
	About              *string `json:"about,omitempty"`
	AvatarMediaID      *string `json:"avatar_media_id,omitempty"`
	LastSeenVisibility *string `json:"last_seen_visibility,omitempty"`
	// ContactPhone: nil = leave unchanged, "" = remove, otherwise a number in E.164 (unverified).
	ContactPhone *string `json:"contact_phone,omitempty"`
}

type UpdateUsernameRequest struct {
	Username string `json:"username"`
}

type ReportUserRequest struct {
	Reason           string `json:"reason"`
	ContextMessageID string `json:"context_message_id,omitempty"`
}

type SearchResponse struct {
	Users []PublicProfile `json:"users"`
}
