package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Resend signs webhooks with Svix: HMAC-SHA256 over "<svix-id>.<svix-timestamp>.<body>", keyed with the base64 part of
// the "whsec_..." secret, sent as "v1,<base64>" (several space-separated signatures during key rotation).
const webhookTolerance = 5 * time.Minute

var ErrBadWebhookSignature = errors.New("auth: webhook signature invalid")

func VerifySvix(secret, id, timestamp, signatureHeader string, body []byte, now time.Time) error {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil || len(key) == 0 || id == "" {
		return ErrBadWebhookSignature
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrBadWebhookSignature
	}
	if d := now.Sub(time.Unix(ts, 0)); d > webhookTolerance || d < -webhookTolerance {
		return ErrBadWebhookSignature // replayed or badly skewed
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + timestamp + "."))
	mac.Write(body)
	want := mac.Sum(nil)
	for _, part := range strings.Fields(signatureHeader) {
		v, sig, ok := strings.Cut(part, ",")
		if !ok || v != "v1" {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(sig)
		if err == nil && hmac.Equal(got, want) {
			return nil
		}
	}
	return ErrBadWebhookSignature
}

type resendEvent struct {
	Type string `json:"type"`
	Data struct {
		To     []string `json:"to"`
		Bounce struct {
			Type string `json:"type"` // Permanent | Transient | Undetermined
		} `json:"bounce"`
	} `json:"data"`
}

// suppressionFor maps an event to (reason, addresses) — only permanent bounces and spam complaints suppress.
func suppressionFor(ev resendEvent) (string, []string) {
	switch ev.Type {
	case "email.bounced":
		if !strings.EqualFold(ev.Data.Bounce.Type, "Permanent") {
			return "", nil // soft bounces (full mailbox, etc.) may recover
		}
		return "bounced", ev.Data.To
	case "email.complained":
		return "complained", ev.Data.To
	}
	return "", nil
}

func (r *Repository) AddSuppression(ctx context.Context, email, reason string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO email_suppressions (email, reason) VALUES ($1, $2) ON CONFLICT (email) DO NOTHING`, strings.ToLower(strings.TrimSpace(email)), reason)
	return err
}

func (r *Repository) IsSuppressed(ctx context.Context, email string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM email_suppressions WHERE email = $1)`, strings.ToLower(strings.TrimSpace(email))).Scan(&ok)
	return ok, err
}

// ResendWebhook handles POST /v1/webhooks/resend. Unsigned or stale requests get 401 and do nothing.
func ResendWebhook(secret string, repo *Repository, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := VerifySvix(secret, r.Header.Get("svix-id"), r.Header.Get("svix-timestamp"), r.Header.Get("svix-signature"), body, time.Now()); err != nil {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		var ev resendEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		reason, addrs := suppressionFor(ev)
		for _, a := range addrs {
			if err := repo.AddSuppression(r.Context(), a, reason); err != nil {
				log.Error("webhook: store suppression failed", zap.Error(err)) // 500 → Resend retries
				http.Error(w, "error", http.StatusInternalServerError)
				return
			}
			log.Info("e-mail address suppressed", zap.String("reason", reason))
		}
		w.WriteHeader(http.StatusOK)
	}
}

// ErrSuppressed is returned instead of sending to an address the provider reported as undeliverable.
var ErrSuppressed = errors.New("auth: address is on the suppression list")

// SuppressingMailer skips sends to suppressed addresses and otherwise delegates.
type SuppressingMailer struct {
	Inner Mailer
	Repo  *Repository
}

func (m SuppressingMailer) check(ctx context.Context, to string) error {
	if ok, err := m.Repo.IsSuppressed(ctx, to); err == nil && ok {
		return ErrSuppressed
	}
	return nil // a failed lookup must not block password-reset mail
}

func (m SuppressingMailer) SendPasswordReset(ctx context.Context, to, url string) error {
	if err := m.check(ctx, to); err != nil {
		return err
	}
	return m.Inner.SendPasswordReset(ctx, to, url)
}

func (m SuppressingMailer) SendEmailVerification(ctx context.Context, to, code string) error {
	if err := m.check(ctx, to); err != nil {
		return err
	}
	return m.Inner.SendEmailVerification(ctx, to, code)
}

func (m SuppressingMailer) SendSecurityNotice(ctx context.Context, to string, n SecurityNotice) error {
	if err := m.check(ctx, to); err != nil {
		return err
	}
	if nf, ok := m.Inner.(SecurityNotifier); ok {
		return nf.SendSecurityNotice(ctx, to, n)
	}
	return nil
}
