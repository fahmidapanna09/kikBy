package users

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/response"
)

// maxSettingsBytes bounds the stored blob; real settings are well under 1 KB.
const maxSettingsBytes = 8 * 1024

var ErrInvalidSettings = errors.New("users: settings must be a JSON object under 8 KB")

// ValidateSettings accepts only a non-empty-or-empty JSON object within the size cap.
func ValidateSettings(raw []byte) error {
	if len(raw) == 0 || len(raw) > maxSettingsBytes {
		return ErrInvalidSettings
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ErrInvalidSettings
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return ErrInvalidSettings
	}
	return nil
}

// GetSettings returns the stored object, or nil if the user has never saved any.
func (r *Repository) GetSettings(ctx context.Context, userID string) (json.RawMessage, error) {
	var data []byte
	err := r.pool.QueryRow(ctx, `SELECT data FROM user_settings WHERE user_id = $1`, userID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func (r *Repository) PutSettings(ctx context.Context, userID string, data json.RawMessage) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_settings (user_id, data) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET data = EXCLUDED.data, updated_at = now()`, userID, []byte(data))
	return err
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.repo.GetSettings(r.Context(), userID(r))
	if err != nil {
		h.log.Error("get settings failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to load settings", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"settings": data}) // data is null until first saved
}

func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxSettingsBytes+1))
	if err != nil || ValidateSettings(raw) != nil {
		response.Error(w, http.StatusBadRequest, "invalid_body", ErrInvalidSettings.Error(), requestID(r))
		return
	}
	if err := h.svc.repo.PutSettings(r.Context(), userID(r), json.RawMessage(raw)); err != nil {
		h.log.Error("put settings failed", zap.Error(err), zap.String("request_id", requestID(r)))
		response.Error(w, http.StatusInternalServerError, "internal_error", "failed to save settings", requestID(r))
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"settings": json.RawMessage(raw)})
}
