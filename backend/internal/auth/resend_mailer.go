package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

const resendDefaultBaseURL = "https://api.resend.com"

// ResendMailer sends transactional e-mail through the Resend HTTP API (POST /emails).
// Only the standard library is used. The API key is sent as a bearer token and is never logged or put in errors.
type ResendMailer struct {
	APIKey  string
	From    string // "KikBite <noreply@your-verified-domain>" — the domain must be verified in Resend
	BaseURL string // override for tests; defaults to https://api.resend.com
	HTTP    *http.Client
	// retryWait is the pause before the single retry (0 → 500 ms); tests set it tiny.
	retryWait time.Duration
}

func NewResendMailer(apiKey, from string) *ResendMailer {
	return &ResendMailer{APIKey: apiKey, From: from, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
	Text    string   `json:"text"`
}

func (m *ResendMailer) SendPasswordReset(ctx context.Context, toEmail, resetURL string) error {
	return m.send(ctx, "reset", toEmail, "Reset your KikBite password",
		"We got a request to reset your KikBite password.\n\nChoose a new one here (valid for 30 minutes):\n"+resetURL+"\n\nIf this wasn't you, ignore this e-mail — your password stays the same.\n",
		renderEmail(emailSpec{
			Preheader: "Use this link to choose a new password. It expires in 30 minutes.",
			Title:     "Reset your password",
			Intro:     "We got a request to reset the password for your KikBite account. Click the button below to choose a new one.",
			Button:    "Choose a new password",
			Link:      resetURL,
			Expiry:    "This link expires in 30 minutes and can be used once.",
			Tip:       "KikBite will never ask for your password by e-mail, chat or phone.",
			Footer:    "If you didn't ask for this, you can safely ignore this e-mail — your password stays the same.",
			ToEmail:   toEmail,
		}), resetURL)
}

func (m *ResendMailer) SendEmailVerification(ctx context.Context, toEmail, code string) error {
	minutes := fmt.Sprintf("%d minutes", int(verifyCodeTTL.Minutes()))
	// The idempotency key only de-duplicates the automatic retry of THIS send. A code could repeat by chance, so the time
	// is part of the key: a genuinely new request must never be swallowed as a "duplicate" of an earlier one.
	nonce := code + "|" + time.Now().UTC().Format(time.RFC3339Nano)
	return m.send(ctx, "verify", toEmail, "Your KikBite verification code",
		"Your KikBite verification code is "+code+".\n\nIt expires in "+minutes+". Never share this code with anyone — KikBite will never ask you for it.\n\nIf you didn't request it, ignore this e-mail.\n",
		renderEmail(emailSpec{
			Preheader: "Your code is " + code + ". It expires in " + minutes + ".",
			Title:     "Confirm your e-mail address",
			Intro:     "Enter this code in KikBite to confirm your e-mail address.",
			Code:      code,
			Expiry:    "This code expires in " + minutes + ".",
			Tip:       "Never share this code. KikBite will never ask you for it by chat, phone or e-mail.",
			Footer:    "If you didn't request this, you can ignore this e-mail and nothing will happen.",
			ToEmail:   toEmail,
		}), nonce)
}

// idempotencyKey makes the one automatic retry safe: Resend de-duplicates by this key for 24 h, so a timeout
// followed by a retry can never deliver two copies. The link carries a fresh random token, so the key is unique per mail.
func idempotencyKey(kind, to, link string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + to + "\x00" + link))
	return "kikbite-" + kind + "-" + hex.EncodeToString(sum[:16])
}

func (m *ResendMailer) send(ctx context.Context, kind, to, subject, text, htmlBody, link string) error {
	if m.APIKey == "" || m.From == "" {
		return ErrMailerNotConfigured
	}
	payload, err := json.Marshal(resendRequest{From: m.From, To: []string{to}, Subject: subject, HTML: htmlBody, Text: text})
	if err != nil {
		return err
	}
	key := idempotencyKey(kind, to, link)
	wait := m.retryWait
	if wait == 0 {
		wait = 500 * time.Millisecond
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
		retry, err := m.post(ctx, payload, key)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return lastErr
}

// post performs one request. retry=true means the failure is transient (network, 429, 5xx).
func (m *ResendMailer) post(ctx context.Context, payload []byte, idemKey string) (retry bool, err error) {
	base := strings.TrimRight(m.BaseURL, "/")
	if base == "" {
		base = resendDefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/emails", bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idemKey)
	client := m.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		// The *url.Error text contains the request URL but never the headers, so the key cannot leak here.
		return ctx.Err() == nil, fmt.Errorf("resend: request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return false, nil
	}
	var apiErr struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &apiErr)
	msg := apiErr.Message
	if msg == "" {
		msg = strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
	}
	e := fmt.Errorf("resend: status %d (%s): %s", resp.StatusCode, apiErr.Name, msg)
	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500, e
}

func (m *ResendMailer) SendSecurityNotice(ctx context.Context, toEmail string, n SecurityNotice) error {
	subject, text := noticeText(n)
	if subject == "" {
		return nil
	}
	body := `<!doctype html><html><body style="font-family:-apple-system,Segoe UI,Roboto,Arial,sans-serif;color:#1b1d21;max-width:480px;margin:24px auto;padding:0 16px">` +
		`<p style="font-weight:700;font-size:18px;margin:0 0 12px">KikBite</p><h1 style="font-size:20px">` + html.EscapeString(subject) + `</h1>` +
		`<p style="line-height:1.5;color:#444">` + strings.ReplaceAll(html.EscapeString(text), "\n\n", "</p><p style=\"line-height:1.5;color:#444\">") + `</p></body></html>`
	// the idempotency key only de-duplicates the automatic retry, so it must differ per notice: add the time
	return m.send(ctx, "notice-"+n.Kind, toEmail, subject, text, body, time.Now().UTC().Format(time.RFC3339Nano))
}
