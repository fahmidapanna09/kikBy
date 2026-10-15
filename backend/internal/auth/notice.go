package auth

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Security notices tell the owner of an address that something sensitive happened, so a takeover is noticed
// even when the attacker is quick. They carry no link and no secret.
const (
	NoticePasswordChanged     = "password_changed"
	NoticePasswordReset       = "password_reset"
	NoticeEmailChangeRequired = "email_change_requested" // Detail = the (masked) address it would move to
	NoticeEmailChanged        = "email_changed"
)

type SecurityNotice struct {
	Kind   string
	Detail string
}

// SecurityNotifier is optional: a Mailer that also implements it gets notices, others silently don't.
type SecurityNotifier interface {
	SendSecurityNotice(ctx context.Context, toEmail string, n SecurityNotice) error
}

// maskEmail keeps the first character and the domain: "alex@example.com" -> "a***@example.com".
func maskEmail(e string) string {
	at := strings.LastIndex(e, "@")
	if at < 1 {
		return "***"
	}
	return e[:1] + "***" + e[at:]
}

func noticeText(n SecurityNotice) (subject, text string) {
	switch n.Kind {
	case NoticePasswordChanged:
		return "Your KikBite password was changed", "The password on your KikBite account was just changed and your other devices were signed out.\n\nIf this was you, nothing more to do. If it wasn't, reset your password right away and check your account."
	case NoticePasswordReset:
		return "Your KikBite password was reset", "Your KikBite password was just reset using an e-mail link and all devices were signed out.\n\nIf this wasn't you, reset your password again and check your account."
	case NoticeEmailChangeRequired:
		return "KikBite: e-mail change requested", "Someone asked to change the sign-in e-mail of your KikBite account to " + n.Detail + ".\n\nNothing changes until that address confirms. If this wasn't you, change your password now."
	case NoticeEmailChanged:
		return "The e-mail on your KikBite account was changed", "The sign-in e-mail of your KikBite account was just changed away from this address, and the new address was confirmed.\n\nIf this wasn't you, contact support immediately — someone with access to your account made this change."
	}
	return "", ""
}

// notifyAsync sends a notice without delaying or failing the request that caused it.
func (s *Service) notifyAsync(toEmail string, n SecurityNotice) {
	if toEmail == "" || s.email.Mailer == nil {
		return
	}
	nf, ok := s.email.Mailer.(SecurityNotifier)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := nf.SendSecurityNotice(ctx, toEmail, n); err != nil && s.email.Log != nil {
			s.email.Log.Warn("security notice not sent", zap.String("kind", n.Kind), zap.Error(err))
		}
	}()
}
