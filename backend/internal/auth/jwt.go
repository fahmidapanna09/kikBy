package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("auth: invalid token")

type AccessClaims struct {
	UserID   string `json:"uid"`
	DeviceID string `json:"did"`
	jwt.RegisteredClaims
}

type JWTService struct {
	accessSecret  []byte
	refreshSecret []byte // reserved for future use if refresh tokens become JWTs; currently opaque random tokens
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

func NewJWTService(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *JWTService {
	return &JWTService{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
}

// IssueAccessToken creates a short-lived signed JWT. Access tokens are
// stateless — they are never stored server-side, which is why they must
// stay short-lived (default 15 min) and revocation happens by revoking the
// underlying session so refresh is blocked, not by an access-token blocklist.
func (s *JWTService) IssueAccessToken(userID, deviceID string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(s.accessTTL)
	claims := AccessClaims{
		UserID:   userID,
		DeviceID: deviceID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Subject:   userID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.accessSecret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func (s *JWTService) VerifyAccessToken(tokenStr string) (userID, deviceID string, err error) {
	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.accessSecret, nil
	})
	if err != nil || !token.Valid {
		return "", "", ErrInvalidToken
	}
	return claims.UserID, claims.DeviceID, nil
}

// GenerateRefreshToken returns a random opaque token (given to the client)
// plus its SHA-256 hash (stored server-side in user_sessions). We never
// store the raw refresh token, mirroring how passwords are handled.
func GenerateRefreshToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	hash = HashRefreshToken(raw)
	return raw, hash, nil
}

func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *JWTService) RefreshTTL() time.Duration { return s.refreshTTL }
