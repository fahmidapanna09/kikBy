package notifications

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrInvalidToken means FCM says this registration token is dead (app uninstalled, data cleared…).
// The worker deletes the token instead of retrying.
var ErrInvalidToken = errors.New("notifications: push token is no longer valid")

const (
	fcmScope       = "https://www.googleapis.com/auth/firebase.messaging"
	googleTokenURI = "https://oauth2.googleapis.com/token"
)

type serviceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// FCMSender delivers pushes through Firebase Cloud Messaging's HTTP v1 API. It needs no extra libraries: the
// service-account key signs a short-lived JWT (RS256) that is exchanged for an OAuth access token, which is cached.
type FCMSender struct {
	sa   serviceAccount
	key  *rsa.PrivateKey
	http *http.Client

	sendBase string // https://fcm.googleapis.com (overridable in tests)

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// NewFCMSenderFromJSON accepts the service-account JSON itself, or the same JSON base64-encoded
// (handy when an env-var editor mangles quotes and newlines).
func NewFCMSenderFromJSON(raw string) (*FCMSender, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("fcm: empty service account")
	}
	if !strings.HasPrefix(raw, "{") {
		dec, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("fcm: service account is neither JSON nor base64: %w", err)
		}
		raw = string(dec)
	}
	var sa serviceAccount
	if err := json.Unmarshal([]byte(raw), &sa); err != nil {
		return nil, fmt.Errorf("fcm: bad service account JSON: %w", err)
	}
	if sa.ProjectID == "" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("fcm: service account JSON needs project_id, client_email and private_key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = googleTokenURI
	}
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return nil, errors.New("fcm: private_key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("fcm: parse private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("fcm: private key is not RSA")
	}
	return &FCMSender{sa: sa, key: key, http: &http.Client{Timeout: 15 * time.Second}, sendBase: "https://fcm.googleapis.com"}, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (f *FCMSender) accessToken(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.token != "" && time.Until(f.tokenExp) > 2*time.Minute {
		return f.token, nil
	}
	now := time.Now()
	claims, err := json.Marshal(map[string]any{
		"iss": f.sa.ClientEmail, "scope": fcmScope, "aud": f.sa.TokenURI,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}
	signing := b64([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + b64(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {signing + "." + b64(sig)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.sa.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fcm: token exchange failed: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", errors.New("fcm: token exchange returned no access_token")
	}
	f.token = out.AccessToken
	f.tokenExp = now.Add(time.Duration(out.ExpiresIn) * time.Second)
	return f.token, nil
}

// describePush turns an outbox row into what the phone shows, and where tapping it should go.
func describePush(kind string, payload []byte) (title, body, path string) {
	var p struct {
		ConversationID string `json:"conversation_id"`
		SenderName     string `json:"sender_name"`
		Preview        string `json:"preview"`
	}
	_ = json.Unmarshal(payload, &p)
	name := p.SenderName
	if name == "" {
		name = "KikBite"
	}
	path = "/"
	if p.ConversationID != "" {
		path = "/chats/" + p.ConversationID
	}
	switch kind {
	case "message.new":
		body = p.Preview
		if body == "" {
			body = "New message"
		}
		return name, body, path
	case "call.incoming":
		return "Incoming call", name + " is calling you", path
	default:
		return "KikBite", "You have a new notification", path
	}
}

// androidConfig: messages use the normal channel; an incoming call uses the loud "calls" channel (heads-up, sound,
// vibration, visible on the lock screen) and expires after 30 s, so nobody gets "ringing" for a call that is long over.
func androidConfig(kind string) map[string]any {
	if kind == "call.incoming" {
		return map[string]any{
			"priority": "HIGH",
			"ttl":      "30s",
			"notification": map[string]any{
				"channel_id":              "kikbite_calls",
				"sound":                   "default",
				"default_vibrate_timings": true,
				"notification_priority":   "PRIORITY_MAX",
				"visibility":              "PUBLIC",
			},
		}
	}
	return map[string]any{"priority": "HIGH", "notification": map[string]any{"channel_id": "kikbite"}}
}

// Send delivers one push. 404/UNREGISTERED and invalid-token answers become ErrInvalidToken.
func (f *FCMSender) Send(ctx context.Context, fcmToken, kind string, payload []byte) error {
	access, err := f.accessToken(ctx)
	if err != nil {
		return err
	}
	title, body, path := describePush(kind, payload)
	msg := map[string]any{"message": map[string]any{
		"token":        fcmToken,
		"notification": map[string]string{"title": title, "body": body},
		"data":         map[string]string{"kind": kind, "path": path},
		"android":      androidConfig(kind),
	}}
	buf, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	endpoint := f.sendBase + "/v1/projects/" + url.PathEscape(f.sa.ProjectID) + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	switch {
	case resp.StatusCode == http.StatusOK:
		return nil
	case resp.StatusCode == http.StatusNotFound || strings.Contains(string(rb), "UNREGISTERED"):
		return ErrInvalidToken
	case resp.StatusCode == http.StatusBadRequest && strings.Contains(string(rb), "registration token"):
		return ErrInvalidToken
	case resp.StatusCode == http.StatusUnauthorized:
		f.mu.Lock()
		f.token = "" // force a fresh access token on the retry
		f.mu.Unlock()
	}
	return fmt.Errorf("fcm: send failed: %d %s", resp.StatusCode, strings.TrimSpace(string(rb)))
}
