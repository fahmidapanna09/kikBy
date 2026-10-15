package response

import (
	"encoding/json"
	"net/http"
)

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

type errorEnvelope struct {
	Error ErrorBody `json:"error"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes a standard error envelope. code is a stable machine-readable
// string (e.g. "invalid_otp", "rate_limited") that clients can switch on;
// message is human-readable and safe to show in logs but should not leak
// internal details to the client in production.
func Error(w http.ResponseWriter, status int, code, message, requestID string) {
	JSON(w, status, errorEnvelope{Error: ErrorBody{Code: code, Message: message, RequestID: requestID}})
}
