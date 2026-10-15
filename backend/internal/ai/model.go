// Package ai implements the Gemini-backed AI agent (Phase 9).
//
// Hard isolation rule (spec section 7 + roadmap Phase 9 exit criteria
// "no auto-access to private chats"): this package imports nothing from
// chat, groups, communities, channels, stories, or contacts, and its
// repository never queries any table outside ai_conversations/ai_messages.
// The AI agent only ever sees what the user typed directly into it. If a
// future feature wants the agent to "see" a chat (e.g. summarize a
// conversation on request), that must be an explicit, user-initiated,
// per-request data pass-in at the handler layer — never a standing
// capability of this service.
package ai

import (
	"errors"
	"time"
)

var (
	ErrNotFound       = errors.New("ai: not found")
	ErrEmptyMessage   = errors.New("ai: message content is required")
	ErrRateLimited    = errors.New("ai: rate limited")
	ErrUpstream       = errors.New("ai: upstream model error")
	ErrMessageTooLong = errors.New("ai: message exceeds maximum length")
)

// MaxUserMessageChars bounds a single user turn. Gemini itself has a much
// larger context window, but an unbounded message is a cheap way for one
// user to burn the whole module's quota/cost budget in one request.
const MaxUserMessageChars = 8000

type Role string

const (
	RoleUser  Role = "user"
	RoleModel Role = "model"
)

type Conversation struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           Role      `json:"role"`
	Content        string    `json:"content"`
	FinishReason   *string   `json:"finish_reason,omitempty"`
	Error          *string   `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type CreateConversationRequest struct {
	Title string `json:"title"`
}

type SendMessageRequest struct {
	Content string `json:"content"`
}

type UpdateConversationRequest struct {
	Title string `json:"title"`
}
