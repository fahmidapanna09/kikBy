package auth

import "time"

type User struct {
	ID          string
	PhoneNumber string
	CountryCode string
	Username    *string
	Email       string
	Status      string
	CreatedAt   time.Time
	// EmailVerified is only populated by GetCredentialsByIdentifier (login).
	EmailVerified bool
}

type Device struct {
	ID         string
	UserID     string
	DeviceID   string
	Platform   string
	PushToken  *string
	AppVersion *string
}

type Session struct {
	ID            string
	UserID        string
	DeviceID      string
	IPAddress     string
	UserAgent     string
	IssuedAt      time.Time
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	CurrentDevice bool
}

// --- Request DTOs ---

type RequestOTPRequest struct {
	PhoneNumber string `json:"phone_number"` // E.164
	Purpose     string `json:"purpose"`      // login | phone_change | account_recovery
}

type VerifyOTPRequest struct {
	PhoneNumber string `json:"phone_number"`
	Code        string `json:"code"`
	DeviceID    string `json:"device_id"`
	Platform    string `json:"platform"` // android | ios | web
	PushToken   string `json:"push_token,omitempty"`
	AppVersion  string `json:"app_version,omitempty"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// --- Response DTOs ---

type TokenPair struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type AuthResponse struct {
	User   PublicUser `json:"user"`
	Tokens TokenPair  `json:"tokens"`
	IsNew  bool       `json:"is_new_user"`
}

type PublicUser struct {
	ID          string  `json:"id"`
	Email       string  `json:"email,omitempty"`
	PhoneNumber string  `json:"phone_number,omitempty"`
	Username    *string `json:"username,omitempty"`
	// EmailVerified reports whether the address was confirmed with the e-mailed code (not enforced at login yet).
	EmailVerified bool `json:"email_verified"`
}

type SessionInfo struct {
	ID        string    `json:"id"`
	Platform  string    `json:"platform"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	IssuedAt  time.Time `json:"issued_at"`
	IsCurrent bool      `json:"is_current"`
}

// --- Email + password accounts (migration 0012) ---

type RegisterRequest struct {
	Name         string `json:"name"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	ContactPhone string `json:"phone,omitempty"` // optional, UNVERIFIED, display only
	DeviceID     string `json:"device_id"`
	Platform     string `json:"platform"`
	AppVersion   string `json:"app_version,omitempty"`
}

type LoginRequest struct {
	Identifier string `json:"identifier"` // e-mail or username
	Password   string `json:"password"`
	DeviceID   string `json:"device_id"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version,omitempty"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// FieldErrors maps a request field name to a human-readable message.
type FieldErrors map[string]string

type VerifyEmailRequest struct {
	Code string `json:"code"` // the 6 digits from the e-mail
}
