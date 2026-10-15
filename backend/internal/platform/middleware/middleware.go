package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/response"
)

type ctxKey string

const (
	CtxRequestID ctxKey = "request_id"
	CtxUserID    ctxKey = "user_id"
	CtxDeviceID  ctxKey = "device_id"
)

// RequestID stamps every request with a correlation ID used across logs,
// error responses, and downstream calls.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), CtxRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Recover converts panics into a 500 response instead of crashing the
// process, and logs the panic without ever including request bodies
// (which may contain message content).
func Recover(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					reqID, _ := r.Context().Value(CtxRequestID).(string)
					log.Error("panic recovered", zap.Any("panic", rec), zap.String("request_id", reqID), zap.String("path", r.URL.Path))
					response.Error(w, http.StatusInternalServerError, "internal_error", "something went wrong", reqID)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets the baseline response headers from spec section 7
// ("Transport: ... HSTS, secure headers (CSP, X-Frame-Options, etc.)").
// This is a JSON API, not a page-serving app, so the CSP is maximally
// restrictive (default-src 'none') rather than tuned for scripts/styles —
// it exists mainly so that if any endpoint ever accidentally reflects
// attacker-controlled content (an error message, a media proxy response),
// a browser that somehow renders the response won't execute anything from
// it. HSTS is only sent when the request arrived over TLS (or behind a
// load balancer that says it did via X-Forwarded-Proto) — sending it over
// plain HTTP local dev would otherwise get cached by a browser and break
// http://localhost testing.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")

		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			// 2 years, include subdomains — standard HSTS preload-eligible
			// value. Not submitted to the preload list here; that's a
			// deployment-time decision, not something to bake into source.
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

// TokenVerifier is satisfied by the auth module's JWT service, kept as an
// interface here so this middleware package has no dependency on auth.
type TokenVerifier interface {
	VerifyAccessToken(token string) (userID string, deviceID string, err error)
}

// RequireAuth validates the bearer access token and injects user/device IDs
// into the request context. It never trusts any client-supplied identity
// header — only the verified JWT claims.
func RequireAuth(verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID, _ := r.Context().Value(CtxRequestID).(string)
			authHeader := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if len(authHeader) <= len(prefix) || authHeader[:len(prefix)] != prefix {
				response.Error(w, http.StatusUnauthorized, "missing_token", "authorization header required", reqID)
				return
			}
			token := authHeader[len(prefix):]
			userID, deviceID, err := verifier.VerifyAccessToken(token)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid or expired", reqID)
				return
			}
			ctx := context.WithValue(r.Context(), CtxUserID, userID)
			ctx = context.WithValue(ctx, CtxDeviceID, deviceID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
