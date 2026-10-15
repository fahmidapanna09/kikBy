package chat

import (
	"context"
	"errors"
	"strings"
)

var ErrEmptySearchQuery = errors.New("chat: search query must not be empty")

func (s *Service) SearchMessages(ctx context.Context, userID, query string, conversationID *string) (SearchResponse, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return SearchResponse{}, ErrEmptySearchQuery
	}
	// If narrowed to one conversation, confirm membership up front so the
	// error is a clean 403 rather than the search just silently returning
	// zero rows (the repository query also scopes by membership, but this
	// gives a more honest error for the single-conversation case).
	if conversationID != nil {
		isMember, err := s.repo.IsMember(ctx, *conversationID, userID)
		if err != nil {
			return SearchResponse{}, err
		}
		if !isMember {
			return SearchResponse{}, ErrNotAMember
		}
	}

	messages, err := s.repo.SearchMessages(ctx, userID, trimmed, conversationID, 50)
	if err != nil {
		return SearchResponse{}, err
	}
	return SearchResponse{Messages: messages}, nil
}
