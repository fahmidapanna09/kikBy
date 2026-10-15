package chat

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

const maxEditBodyRunes = 4000

var ErrInvalidBody = errors.New("chat: message body is empty or too long")

// memberOfMessage resolves a message and checks the caller belongs to its conversation.
func (s *Service) memberOfMessage(ctx context.Context, userID, messageID string) (MessageMeta, error) {
	meta, err := s.repo.GetMessageMeta(ctx, messageID)
	if err != nil {
		return MessageMeta{}, err
	}
	isMember, err := s.repo.IsMember(ctx, meta.ConversationID, userID)
	if err != nil {
		return MessageMeta{}, err
	}
	if !isMember {
		return MessageMeta{}, ErrNotAMember
	}
	return meta, nil
}

// EditMessage lets the sender change the text of their own live text message
// and pushes the full updated message to every member (including the sender's
// other devices) so clients replace it in place.
func (s *Service) EditMessage(ctx context.Context, userID, messageID, body string) (Message, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > maxEditBodyRunes {
		return Message{}, ErrInvalidBody
	}
	meta, err := s.memberOfMessage(ctx, userID, messageID)
	if err != nil {
		return Message{}, err
	}
	if meta.SenderID != userID {
		return Message{}, ErrNotSender
	}
	if meta.Deleted {
		return Message{}, ErrMessageDeleted
	}
	if meta.Type != "text" {
		return Message{}, ErrNotEditable
	}
	msg, err := s.repo.EditMessageBody(ctx, messageID, body)
	if err != nil {
		return Message{}, err
	}
	env := NewEnvelope(EventMessageEdited, msg.ConversationID, "", MessageEditedPayload{Message: msg})
	s.broadcastToMembers(ctx, msg.ConversationID, env)
	return msg, nil
}

// DeleteMessage removes a message for the caller only (anyone in the
// conversation may do that) or for everyone (sender only).
func (s *Service) DeleteMessage(ctx context.Context, userID, messageID string, everyone bool) error {
	meta, err := s.memberOfMessage(ctx, userID, messageID)
	if err != nil {
		return err
	}
	if !everyone {
		return s.repo.DeleteMessageForUser(ctx, messageID, userID)
	}
	if meta.SenderID != userID {
		return ErrNotSender
	}
	if err := s.repo.DeleteMessageForEveryone(ctx, messageID); err != nil {
		return err
	}
	env := NewEnvelope(EventMessageDeleted, meta.ConversationID, "", MessageDeletedPayload{MessageID: messageID})
	s.broadcastToMembers(ctx, meta.ConversationID, env)
	return nil
}

func (s *Service) SetPinned(ctx context.Context, userID, conversationID string, on bool) error {
	if ok, err := s.repo.IsMember(ctx, conversationID, userID); err != nil {
		return err
	} else if !ok {
		return ErrNotAMember
	}
	return s.repo.SetPinned(ctx, conversationID, userID, on)
}

func (s *Service) SetArchived(ctx context.Context, userID, conversationID string, on bool) error {
	if ok, err := s.repo.IsMember(ctx, conversationID, userID); err != nil {
		return err
	} else if !ok {
		return ErrNotAMember
	}
	return s.repo.SetArchived(ctx, conversationID, userID, on)
}
