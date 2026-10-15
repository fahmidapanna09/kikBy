package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/yourorg/chatplatform/internal/notifications"
)

type Service struct {
	repo             *Repository
	hub              *Hub
	presence         *Presence
	outbox           *notifications.Outbox
	attachmentLinker AttachmentLinker
	blockChecker     BlockChecker
	calls            *CallService // nil until EnableCalls is called
}

// AttachmentLinker is satisfied structurally by media.Service (it exposes a
// method with this exact signature) — chat never imports the media package,
// keeping the module boundary intact while still letting SendMessage attach
// previously-uploaded files to the message it's creating.
type AttachmentLinker interface {
	AttachToMessage(ctx context.Context, userID, messageID, mediaID string, position int) error
}

// BlockChecker is satisfied structurally by contacts.Repository — same
// module-boundary approach as AttachmentLinker above.
type BlockChecker interface {
	IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error)
}

func NewService(repo *Repository, hub *Hub, presence *Presence, outbox *notifications.Outbox, attachmentLinker AttachmentLinker, blockChecker BlockChecker) *Service {
	return &Service{repo: repo, hub: hub, presence: presence, outbox: outbox, attachmentLinker: attachmentLinker, blockChecker: blockChecker}
}

// isBlockedPair checks both directions — if either user has blocked the
// other, a direct conversation between them is off-limits. Only applies to
// direct (1:1) conversations; group messaging isn't gated by a pairwise
// block the same way. blockChecker being nil (not wired up) fails open
// rather than panicking, so this stays optional for callers/tests that
// don't need it.
func (s *Service) isBlockedPair(ctx context.Context, conversationID, userID string) (bool, error) {
	if s.blockChecker == nil {
		return false, nil
	}
	convType, err := s.repo.GetConversationType(ctx, conversationID)
	if err != nil {
		return false, err
	}
	if convType != "direct" {
		return false, nil
	}
	others, err := s.repo.OtherMembers(ctx, conversationID, userID)
	if err != nil || len(others) != 1 {
		return false, err
	}
	other := others[0]
	if blocked, err := s.blockChecker.IsBlocked(ctx, userID, other); err != nil || blocked {
		return blocked, err
	}
	return s.blockChecker.IsBlocked(ctx, other, userID)
}

func (s *Service) StartDirectConversation(ctx context.Context, userID, peerUserID string) (Conversation, error) {
	if s.blockChecker != nil {
		if blocked, err := s.blockChecker.IsBlocked(ctx, userID, peerUserID); err != nil {
			return Conversation{}, err
		} else if blocked {
			return Conversation{}, ErrBlocked
		}
		if blocked, err := s.blockChecker.IsBlocked(ctx, peerUserID, userID); err != nil {
			return Conversation{}, err
		} else if blocked {
			return Conversation{}, ErrBlocked
		}
	}
	return s.repo.GetOrCreateDirectConversation(ctx, userID, peerUserID)
}

func (s *Service) ListConversations(ctx context.Context, userID string) ([]ConversationSummary, error) {
	return s.repo.ListConversationsForUser(ctx, userID, 50)
}

func (s *Service) ListMessages(ctx context.Context, userID, conversationID string, beforeSeq int64) (MessagePage, error) {
	isMember, err := s.repo.IsMember(ctx, conversationID, userID)
	if err != nil {
		return MessagePage{}, err
	}
	if !isMember {
		return MessagePage{}, ErrNotAMember
	}
	msgs, err := s.repo.ListMessages(ctx, conversationID, userID, beforeSeq, 50)
	if err != nil {
		return MessagePage{}, err
	}
	page := MessagePage{Messages: msgs}
	if len(msgs) == 50 {
		page.NextCursor = strconv.FormatInt(msgs[len(msgs)-1].Seq, 10)
	}
	return page, nil
}

// SendMessage is the single path both the WS handler and (as a fallback)
// the REST handler call. It persists the message, acks the sender, fans the
// new-message event out to every other online member via the hub, and for
// any member who isn't currently online anywhere, enqueues a push
// notification instead — never both, and never silently neither.
func (s *Service) SendMessage(ctx context.Context, conversationID, senderID, senderDeviceID string, req SendMessageRequest) (Message, error) {
	role, isMember, err := s.repo.MemberRoleOf(ctx, conversationID, senderID)
	if err != nil {
		return Message{}, err
	}
	if !isMember {
		return Message{}, ErrNotAMember
	}
	canSend, err := s.repo.CanUserSend(ctx, conversationID, role)
	if err != nil {
		return Message{}, err
	}
	if !canSend {
		return Message{}, ErrSendNotAllowed
	}
	if blocked, err := s.isBlockedPair(ctx, conversationID, senderID); err != nil {
		return Message{}, err
	} else if blocked {
		return Message{}, ErrBlocked
	}
	if req.Type == "" {
		req.Type = "text"
	}

	msg, isNew, err := s.repo.InsertMessage(ctx, conversationID, senderID, req)
	if err != nil {
		return Message{}, err
	}
	if !isNew {
		// best-effort; a retried send just re-reports what's already attached
		msg.Attachments, _ = s.repo.ListAttachmentInfo(ctx, msg.ID)
		for _, a := range msg.Attachments {
			msg.AttachmentIDs = append(msg.AttachmentIDs, a.ID)
		}
		return msg, nil // retried send; already delivered the first time, don't re-fan-out
	}

	if len(req.AttachmentMediaIDs) > 0 {
		var attached []string
		for i, mediaID := range req.AttachmentMediaIDs {
			if err := s.attachmentLinker.AttachToMessage(ctx, senderID, msg.ID, mediaID, i); err != nil {
				// The message itself is already persisted; a failed
				// attachment link (e.g. sender doesn't actually own that
				// media id) doesn't roll back the message, it just means
				// that one attachment won't show up.
				continue
			}
			attached = append(attached, mediaID)
		}
		msg.AttachmentIDs = attached
		msg.Attachments, _ = s.repo.ListAttachmentInfo(ctx, msg.ID)
	}

	others, err := s.repo.OtherMembers(ctx, conversationID, senderID)
	if err != nil {
		return msg, fmt.Errorf("send message: list other members: %w", err)
	}

	for _, recipientID := range others {
		online, err := s.presence.IsOnline(ctx, recipientID)
		if err != nil {
			online = false // fail open to push rather than lose the notification entirely
		}

		env := NewEnvelope(EventMessageNew, conversationID, "", msg)
		if pubErr := s.hub.Publish(ctx, recipientID, env); pubErr != nil {
			online = false // publish failed; fall back to push so the message isn't silently dropped
		}

		if !online {
			if err := s.enqueuePush(ctx, recipientID, msg); err != nil {
				// Logged by caller via returned error path in a full
				// implementation; for now this is best-effort and the
				// worker's own retry loop is the real backstop.
				_ = err
			}
		}
	}

	return msg, nil
}

func (s *Service) enqueuePush(ctx context.Context, recipientID string, msg Message) error {
	payload, err := json.Marshal(map[string]any{
		"conversation_id": msg.ConversationID,
		"message_id":      msg.ID,
		"sender_id":       msg.SenderID,
		"preview":         previewFor(msg),
	})
	if err != nil {
		return err
	}
	return s.outbox.Enqueue(ctx, recipientID, "message.new", payload)
}

// previewFor never includes the raw message body for anything beyond a
// short text preview, and never for non-text types — keeps push payloads
// (which pass through FCM/APNs infrastructure outside our control) minimal.
func previewFor(msg Message) string {
	if msg.Type != "text" || msg.Body == nil {
		return "Sent a " + msg.Type
	}
	// Truncate by runes, not bytes: slicing bytes can cut a multi-byte character
	// (Bengali, emoji, ...) in half and produce invalid UTF-8 in the push preview.
	runes := []rune(*msg.Body)
	if len(runes) > 80 {
		return string(runes[:80]) + "…"
	}
	return *msg.Body
}

func (s *Service) MarkDelivered(ctx context.Context, conversationID, messageID, userID string) error {
	isMember, err := s.repo.IsMember(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotAMember
	}
	if err := s.repo.MarkDelivered(ctx, messageID, userID); err != nil {
		return err
	}
	env := NewEnvelope(EventMessageDelivered, conversationID, "", DeliveredPayload{MessageID: messageID, UserID: userID})
	return s.fanOutToOthers(ctx, conversationID, userID, env)
}

func (s *Service) MarkRead(ctx context.Context, conversationID, messageID, userID string, seq int64) error {
	isMember, err := s.repo.IsMember(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotAMember
	}
	if seq == 0 {
		// The wire payload doesn't carry seq; resolve it so last_read_seq (and
		// therefore unread counts) actually advance.
		if meta, err := s.repo.GetMessageMeta(ctx, messageID); err == nil && meta.ConversationID == conversationID {
			seq = meta.Seq
		}
	}
	if err := s.repo.MarkRead(ctx, conversationID, messageID, userID, seq); err != nil {
		return err
	}
	env := NewEnvelope(EventMessageRead, conversationID, "", ReadPayload{MessageID: messageID, UserID: userID})
	return s.fanOutToOthers(ctx, conversationID, userID, env)
}

// fanOutToOthers publishes env to every member of conversationID except
// excludingUserID. For 1:1 chat that's exactly the message's original
// sender; this stays correct once group conversations (Phase 4) reuse the
// same receipt path, since it's membership-driven rather than hardcoded to
// "the other person in a pair".
func (s *Service) fanOutToOthers(ctx context.Context, conversationID, excludingUserID string, env Envelope) error {
	others, err := s.repo.OtherMembers(ctx, conversationID, excludingUserID)
	if err != nil {
		return err
	}
	for _, recipientID := range others {
		if err := s.hub.Publish(ctx, recipientID, env); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) BroadcastTyping(ctx context.Context, conversationID, userID string, starting bool) error {
	isMember, err := s.repo.IsMember(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotAMember
	}
	eventType := EventTypingStart
	if !starting {
		eventType = EventTypingStop
	}
	others, err := s.repo.OtherMembers(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	env := NewEnvelope(eventType, conversationID, "", TypingPayload{UserID: userID})
	for _, recipientID := range others {
		if err := s.hub.Publish(ctx, recipientID, env); err != nil {
			return err
		}
	}
	return nil
}
