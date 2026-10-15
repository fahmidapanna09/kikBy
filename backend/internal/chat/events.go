package chat

import (
	"time"

	"github.com/oklog/ulid/v2"
)

// Event types — mirrors the protocol documented in docs/00-architecture-and-roadmap.md section 6.
const (
	EventMessageSend        = "message.send" // client -> server
	EventMessageAck         = "message.ack"  // server -> sender: client_tmp_id resolved to server id/seq
	EventMessageNew         = "message.new"  // server -> recipient(s)
	EventMessageDelivered   = "message.delivered"
	EventMessageRead        = "message.read"
	EventMessageEdited      = "message.edited"
	EventMessageDeleted     = "message.deleted"
	EventTypingStart        = "typing.start"
	EventTypingStop         = "typing.stop"
	EventPresenceUpdate     = "presence.update"
	EventGroupUpdated       = "group.updated"
	EventGroupMemberAdded   = "group.member_added"
	EventGroupMemberRemoved = "group.member_removed"
	EventMessageReaction    = "message.reaction"
	EventPing               = "ping"
	EventPong               = "pong"
	EventError              = "error"
)

// Envelope is the wire format for every WebSocket frame in both directions.
type Envelope struct {
	EventID        string      `json:"event_id"`
	Type           string      `json:"type"`
	Ts             time.Time   `json:"ts"`
	DeviceID       string      `json:"device_id,omitempty"`
	ConversationID string      `json:"conversation_id,omitempty"`
	Version        int         `json:"version"`
	Payload        interface{} `json:"payload"`
}

func NewEnvelope(eventType, conversationID, deviceID string, payload interface{}) Envelope {
	return Envelope{
		EventID:        ulid.Make().String(),
		Type:           eventType,
		Ts:             time.Now().UTC(),
		DeviceID:       deviceID,
		ConversationID: conversationID,
		Version:        1,
		Payload:        payload,
	}
}

// --- Payload shapes ---

type MessageSendPayload struct {
	ConversationID         string   `json:"conversation_id"`
	ClientTmpID            string   `json:"client_tmp_id"`
	Type                   string   `json:"type"`
	Body                   string   `json:"body,omitempty"`
	ReplyToID              string   `json:"reply_to_id,omitempty"`
	AttachmentMediaIDs     []string `json:"attachment_media_ids,omitempty"`
	ForwardedFromMessageID string   `json:"forwarded_from_message_id,omitempty"`
}

type MessageAckPayload struct {
	ClientTmpID string `json:"client_tmp_id"`
	ServerID    string `json:"server_id"`
	Seq         int64  `json:"seq"`
}

type DeliveredPayload struct {
	MessageID string `json:"message_id"`
	UserID    string `json:"user_id"`
}

type ReadPayload struct {
	MessageID string `json:"message_id"`
	UserID    string `json:"user_id"`
}

type TypingPayload struct {
	UserID string `json:"user_id"`
}

type PresencePayload struct {
	UserID   string     `json:"user_id"`
	Online   bool       `json:"online"`
	LastSeen *time.Time `json:"last_seen,omitempty"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type GroupUpdatedPayload struct {
	Group GroupSummary `json:"group"`
}

type GroupMemberAddedPayload struct {
	Members []Member `json:"members"`
	AddedBy string   `json:"added_by"`
}

type GroupMemberRemovedPayload struct {
	UserID    string `json:"user_id"`
	RemovedBy string `json:"removed_by"` // equals UserID when it was the member leaving on their own
}

type MessageReactionPayload struct {
	MessageID string            `json:"message_id"`
	Reactions []ReactionSummary `json:"reactions"`
}

// MessageReactionSetPayload is the client -> server direction (event type
// EventMessageReaction reused both ways): an empty Emoji means "remove my
// reaction" rather than needing a separate event type.
type MessageReactionSetPayload struct {
	MessageID string `json:"message_id"`
	Emoji     string `json:"emoji"`
}

// MessageEditedPayload carries the full updated message so clients replace in place.
type MessageEditedPayload struct {
	Message Message `json:"message"`
}

type MessageDeletedPayload struct {
	MessageID string `json:"message_id"`
}
