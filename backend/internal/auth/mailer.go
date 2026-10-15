package auth

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

// Mailer abstracts outbound e-mail: a password-reset link and a 6-digit e-mail-verification code.
type Mailer interface {
	SendPasswordReset(ctx context.Context, toEmail, resetURL string) error
	SendEmailVerification(ctx context.Context, toEmail, code string) error
}

// ConsoleMailer logs the reset link / verification code instead of sending it. LOCAL DEV ONLY:
// both are credentials, so they must never be logged outside APP_ENV=local.
type ConsoleMailer struct{ Log *zap.Logger }

func (m ConsoleMailer) SendPasswordReset(_ context.Context, toEmail, resetURL string) error {
	m.Log.Info("password reset e-mail (console mailer, local dev only)", zap.String("to", toEmail), zap.String("url", resetURL))
	return nil
}

func (m ConsoleMailer) SendEmailVerification(_ context.Context, toEmail, code string) error {
	m.Log.Info("verification e-mail (console mailer, local dev only)", zap.String("to", toEmail), zap.String("code", code))
	return nil
}

func (m ConsoleMailer) SendSecurityNotice(_ context.Context, toEmail string, n SecurityNotice) error {
	m.Log.Info("security notice e-mail (console mailer, local dev only)", zap.String("to", toEmail), zap.String("kind", n.Kind))
	return nil
}

// ErrMailerNotConfigured is returned by NoopMailer so a deployment without a
// real provider fails loudly in logs instead of silently dropping reset mail.
var ErrMailerNotConfigured = errors.New("auth: no mail provider configured")

type NoopMailer struct{}

func (NoopMailer) SendPasswordReset(context.Context, string, string) error {
	return ErrMailerNotConfigured
}

func (NoopMailer) SendEmailVerification(context.Context, string, string) error {
	return ErrMailerNotConfigured
}
