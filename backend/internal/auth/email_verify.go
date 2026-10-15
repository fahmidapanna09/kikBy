package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/audit"
)

const (
	verifyCodeTTL         = 10 * time.Minute // how long an e-mailed code stays valid
	verifyCodeMaxAttempts = 5                // wrong guesses allowed per code; the 5th burns it
	verifyResendCooldown  = 60 * time.Second
	mailSendTimeout       = 15 * time.Second
)

var (
	ErrInvalidVerifyCode   = errors.New("auth: verification code is wrong")
	ErrVerifyCodeExpired   = errors.New("auth: verification code expired or none was requested")
	ErrTooManyCodeAttempts = errors.New("auth: too many wrong verification codes")
	ErrVerifyTooSoon       = errors.New("auth: verification e-mail was sent a moment ago")
)

// newVerifyCode returns a uniformly random 6-digit code ("000000"–"999999") from crypto/rand.
func newVerifyCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("verify code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// normalizeVerifyCode accepts what people really type ("123 456", "123-456") and returns the bare 6 digits,
// or "" when the input is not a 6-digit code.
func normalizeVerifyCode(raw string) string {
	digits := make([]byte, 0, 6)
	for i := 0; i < len(raw); i++ {
		switch c := raw[i]; {
		case c >= '0' && c <= '9':
			digits = append(digits, c)
		case c == ' ' || c == '-':
			// separators are fine
		default:
			return ""
		}
	}
	if len(digits) != 6 {
		return ""
	}
	return string(digits)
}

// sendVerificationAsync mails a verification code without making registration wait for (or fail on) the
// mail provider. A failure is only logged; the person can use "resend" from the app.
func (s *Service) sendVerificationAsync(userID, email string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mailSendTimeout)
		defer cancel()
		if err := s.sendVerification(ctx, userID, email); err != nil && s.email.Log != nil {
			s.email.Log.Warn("verification e-mail not sent", zap.String("user_id", userID), zap.Error(err))
		}
	}()
}

// sendVerification creates a fresh code (closing any older open one), stores only its salted hash and mails it.
func (s *Service) sendVerification(ctx context.Context, userID, email string) error {
	code, err := newVerifyCode()
	if err != nil {
		return err
	}
	hash, err := hashOTPCode(code)
	if err != nil {
		return err
	}
	if err := s.repo.CreateVerificationToken(ctx, userID, email, hash, time.Now().Add(verifyCodeTTL)); err != nil {
		return fmt.Errorf("store verification code: %w", err)
	}
	mailer := s.email.Mailer
	if mailer == nil {
		mailer = NoopMailer{}
	}
	return mailer.SendEmailVerification(ctx, email, code)
}

// ResendVerification re-sends a code for the signed-in user: to the pending new address if an e-mail change is
// waiting for confirmation, otherwise to the current address. Already-verified accounts get (true, nil).
// A 60 s cooldown (measured from the previous code) keeps this from being used to spam an inbox.
func (s *Service) ResendVerification(ctx context.Context, userID string) (alreadyVerified bool, err error) {
	email, pending, verified, err := s.repo.GetEmailState(ctx, userID)
	if err != nil {
		return false, err
	}
	target := email
	switch {
	case pending != "":
		target = pending
	case verified:
		return true, nil
	}
	if target == "" {
		return false, ErrNotFound
	}
	if last, ok, err := s.repo.LastVerificationSentAt(ctx, userID); err != nil {
		return false, err
	} else if ok && time.Since(last) < verifyResendCooldown {
		return false, ErrVerifyTooSoon
	}
	return false, s.sendVerification(ctx, userID, target)
}

// VerifyEmailCode checks the 6-digit code for the signed-in user. The code is only ever compared with the newest
// open code of THAT user, so guessing is limited to verifyCodeMaxAttempts tries per code (plus the per-IP rate limit).
func (s *Service) VerifyEmailCode(ctx context.Context, userID, rawCode, ip string) error {
	code := normalizeVerifyCode(rawCode)
	if code == "" {
		return ErrInvalidVerifyCode
	}
	oldEmail, err := s.repo.consumeVerificationCode(ctx, userID, code)
	switch {
	case err == nil:
	case errors.Is(err, ErrVerifyCodeExpired), errors.Is(err, ErrInvalidVerifyCode), errors.Is(err, ErrTooManyCodeAttempts):
		return err
	default:
		return fmt.Errorf("verify email: %w", err)
	}
	if oldEmail != "" {
		s.notifyAsync(oldEmail, SecurityNotice{Kind: NoticeEmailChanged})
	}
	s.audit.Log(ctx, audit.EventEmailVerified, userID, ip, "", nil)
	return nil
}

/* ───────── repository ───────── */

// CreateVerificationToken closes any still-open codes for the user, then stores the new hash.
func (r *Repository) CreateVerificationToken(ctx context.Context, userID, email, codeHash string, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE email_verification_tokens SET used_at = now() WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO email_verification_tokens (user_id, email, token_hash, expires_at) VALUES ($1, $2, $3, $4)`, userID, email, codeHash, expiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) LastVerificationSentAt(ctx context.Context, userID string) (time.Time, bool, error) {
	var t time.Time
	err := r.pool.QueryRow(ctx, `SELECT created_at FROM email_verification_tokens WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`, userID).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	return t, err == nil, err
}

func (r *Repository) GetEmailState(ctx context.Context, userID string) (email, pending string, verified bool, err error) {
	err = r.pool.QueryRow(ctx, `SELECT COALESCE(email,''), COALESCE(pending_email,''), email_verified_at IS NOT NULL FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&email, &pending, &verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, ErrNotFound
	}
	return email, pending, verified, err
}

// consumeVerificationCode checks `code` against the user's newest open, unexpired code, all in ONE transaction.
// The row is locked (FOR UPDATE), so concurrent guesses are serialised and cannot dodge the attempt counter.
//
//   - no open code            → ErrVerifyCodeExpired (expired, already used, or never requested)
//   - wrong code              → attempts+1 is saved; ErrInvalidVerifyCode, or ErrTooManyCodeAttempts when it burns the code
//   - right code, and the code's address is …
//     the current address   → stamp email_verified_at;
//     the pending address   → swap it in as users.email (verified), clear pending_email.
//
// If someone else grabbed the address in the meantime, the unique index rejects the swap and the code is treated as expired.
// oldEmail is the address that was replaced ("" unless the sign-in e-mail was swapped).
func (r *Repository) consumeVerificationCode(ctx context.Context, userID, code string) (oldEmail string, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var id, hash, email string
	var attempts int
	err = tx.QueryRow(ctx, `
		SELECT id, token_hash, email, attempts
		FROM email_verification_tokens
		WHERE user_id = $1 AND used_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`, userID).Scan(&id, &hash, &email, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrVerifyCodeExpired
	}
	if err != nil {
		return "", err
	}

	if !verifyOTPCode(hash, code) {
		attempts++
		if attempts >= verifyCodeMaxAttempts {
			if _, err = tx.Exec(ctx, `UPDATE email_verification_tokens SET attempts = $2, used_at = now() WHERE id = $1`, id, attempts); err != nil {
				return "", err
			}
			if err = tx.Commit(ctx); err != nil {
				return "", err
			}
			return "", ErrTooManyCodeAttempts
		}
		if _, err = tx.Exec(ctx, `UPDATE email_verification_tokens SET attempts = $2 WHERE id = $1`, id, attempts); err != nil {
			return "", err
		}
		if err = tx.Commit(ctx); err != nil {
			return "", err
		}
		return "", ErrInvalidVerifyCode
	}

	if _, err = tx.Exec(ctx, `UPDATE email_verification_tokens SET used_at = now() WHERE id = $1`, id); err != nil {
		return "", err
	}
	// read the current address first (row lock) so a completed swap can report what it replaced
	var current, pending string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(email,''), COALESCE(pending_email,'') FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&current, &pending); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE users SET
		  email             = CASE WHEN pending_email = $2 THEN $2 ELSE email END,
		  email_verified_at = CASE WHEN pending_email = $2 THEN now() ELSE COALESCE(email_verified_at, now()) END,
		  pending_email     = CASE WHEN pending_email = $2 THEN NULL ELSE pending_email END
		WHERE id = $1 AND deleted_at IS NULL AND (email = $2 OR pending_email = $2)
	`, userID, email)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", ErrVerifyCodeExpired
		}
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "", ErrVerifyCodeExpired
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	if pending == email && current != "" && current != email {
		oldEmail = current
	}
	return oldEmail, nil
}
