// Package logger provides a structured logger.
//
// HARD RULE: never pass an OTP code, password, access token, refresh token,
// or message body into any log call anywhere in this codebase. Log IDs,
// sizes, durations, and status/error codes instead. Code review must reject
// any log line that could contain one of those values.
package logger

import "go.uber.org/zap"

func New(env string) (*zap.Logger, error) {
	if env == "production" || env == "staging" {
		return zap.NewProduction()
	}
	return zap.NewDevelopment()
}
