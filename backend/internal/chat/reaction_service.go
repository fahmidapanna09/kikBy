package chat

import "context"

func (s *Service) SetReaction(ctx context.Context, userID, messageID, emoji string) error {
	conversationID, err := s.repo.ConversationIDForMessage(ctx, messageID)
	if err != nil {
		return err
	}
	isMember, err := s.repo.IsMember(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotAMember
	}
	if err := s.repo.SetReaction(ctx, messageID, userID, emoji); err != nil {
		return err
	}
	return s.broadcastReactions(ctx, conversationID, messageID)
}

func (s *Service) RemoveReaction(ctx context.Context, userID, messageID string) error {
	conversationID, err := s.repo.ConversationIDForMessage(ctx, messageID)
	if err != nil {
		return err
	}
	isMember, err := s.repo.IsMember(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotAMember
	}
	if err := s.repo.RemoveReaction(ctx, messageID, userID); err != nil {
		return err
	}
	return s.broadcastReactions(ctx, conversationID, messageID)
}

func (s *Service) ListReactions(ctx context.Context, userID, messageID string) ([]ReactionSummary, error) {
	conversationID, err := s.repo.ConversationIDForMessage(ctx, messageID)
	if err != nil {
		return nil, err
	}
	isMember, err := s.repo.IsMember(ctx, conversationID, userID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, ErrNotAMember
	}
	return s.repo.ListReactions(ctx, messageID)
}

// broadcastReactions sends the full current reaction set for a message
// rather than a delta ("user X added 👍") — simpler for clients to apply
// (replace-in-place) and self-correcting if an intermediate event was ever
// missed, at the cost of a slightly larger payload than a delta would be.
// Goes to every member (via broadcastToMembers, shared with group_service.go)
// including the reactor themselves, so all devices converge on one state.
func (s *Service) broadcastReactions(ctx context.Context, conversationID, messageID string) error {
	reactions, err := s.repo.ListReactions(ctx, messageID)
	if err != nil {
		return err
	}
	env := NewEnvelope(EventMessageReaction, conversationID, "", MessageReactionPayload{MessageID: messageID, Reactions: reactions})
	s.broadcastToMembers(ctx, conversationID, env)
	return nil
}
