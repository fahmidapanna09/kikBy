package ai

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yourorg/chatplatform/internal/platform/ratelimit"
)

// Quota: 30 user turns per hour per user. This is deliberately a request
// quota, not a token/cost quota — good enough for abuse control at this
// stage; a real cost-based budget would need per-token accounting from the
// Gemini response (usageMetadata), which is left for Phase 12 hardening.
const (
	maxTurnsPerHour = 30
	rateLimitWindow = time.Hour
)

// DefaultModel is used for a new conversation when the caller doesn't (and
// the Android client currently never does) specify one. Overridable per
// deployment via GEMINI_MODEL; kept here only as the fallback-of-last-resort.
const DefaultModel = "gemini-2.0-flash"

type Service struct {
	repo    *Repository
	gemini  *GeminiClient
	limiter *ratelimit.Limiter
	model   string
}

func NewService(repo *Repository, gemini *GeminiClient, limiter *ratelimit.Limiter, model string) *Service {
	if model == "" {
		model = DefaultModel
	}
	return &Service{repo: repo, gemini: gemini, limiter: limiter, model: model}
}

func (s *Service) CreateConversation(ctx context.Context, userID string, req CreateConversationRequest) (Conversation, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "New chat"
	}
	return s.repo.CreateConversation(ctx, userID, title, s.model)
}

func (s *Service) Get(ctx context.Context, userID, conversationID string) (Conversation, error) {
	return s.repo.GetOwned(ctx, userID, conversationID)
}

func (s *Service) Mine(ctx context.Context, userID string) ([]Conversation, error) {
	return s.repo.ListConversations(ctx, userID)
}

func (s *Service) UpdateTitle(ctx context.Context, userID, conversationID string, req UpdateConversationRequest) error {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return ErrEmptyMessage
	}
	return s.repo.UpdateTitle(ctx, userID, conversationID, title)
}

func (s *Service) Delete(ctx context.Context, userID, conversationID string) error {
	return s.repo.SoftDelete(ctx, userID, conversationID)
}

// ListMessages re-checks ownership before returning history — the handler
// never trusts conversationID alone, only (userID, conversationID) together.
func (s *Service) ListMessages(ctx context.Context, userID, conversationID string) ([]Message, error) {
	if _, err := s.repo.GetOwned(ctx, userID, conversationID); err != nil {
		return nil, err
	}
	return s.repo.ListMessages(ctx, conversationID)
}

// SendMessage persists the user's turn immediately (so it's never lost even
// if the model call then fails), streams the model's reply via onDelta as
// it arrives, and persists the model's turn (or a recorded error) once the
// stream ends. It returns the final persisted model message.
func (s *Service) SendMessage(ctx context.Context, userID, conversationID string, req SendMessageRequest, onDelta func(string)) (Message, error) {
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return Message{}, ErrEmptyMessage
	}
	if utf8.RuneCountInString(content) > MaxUserMessageChars {
		return Message{}, ErrMessageTooLong
	}

	conv, err := s.repo.GetOwned(ctx, userID, conversationID)
	if err != nil {
		return Message{}, err
	}

	ok, err := s.limiter.Allow(ctx, "ai:turn:"+userID, maxTurnsPerHour, rateLimitWindow)
	if err != nil {
		return Message{}, err
	}
	if !ok {
		return Message{}, ErrRateLimited
	}

	history, err := s.repo.ListMessages(ctx, conversationID)
	if err != nil {
		return Message{}, err
	}

	if _, err := s.repo.AppendMessage(ctx, conversationID, RoleUser, content, nil, nil); err != nil {
		return Message{}, err
	}

	geminiHistory := make([]geminiContent, 0, len(history)+1)
	for _, m := range history {
		geminiHistory = append(geminiHistory, geminiContent{
			Role:  string(m.Role),
			Parts: []geminiPart{{Text: m.Content}},
		})
	}
	geminiHistory = append(geminiHistory, geminiContent{
		Role:  string(RoleUser),
		Parts: []geminiPart{{Text: content}},
	})

	modelToUse := conv.Model
	if modelToUse == "" {
		modelToUse = s.model
	}

	result, genErr := s.gemini.StreamGenerate(ctx, modelToUse, geminiHistory, onDelta)
	if genErr != nil {
		errText := genErr.Error()
		msg, saveErr := s.repo.AppendMessage(ctx, conversationID, RoleModel, result.Text, nil, &errText)
		if saveErr != nil {
			return Message{}, saveErr
		}
		return msg, genErr
	}

	var finishReason *string
	if result.FinishReason != "" {
		fr := result.FinishReason
		finishReason = &fr
	}
	return s.repo.AppendMessage(ctx, conversationID, RoleModel, result.Text, finishReason, nil)
}
