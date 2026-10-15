package chat

import "time"

type Conversation struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
}

type Message struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id"`
	Seq            int64      `json:"seq"`
	SenderID       string     `json:"sender_id"`
	ClientTmpID    string     `json:"client_tmp_id"`
	Type           string     `json:"type"`
	Body           *string    `json:"body,omitempty"`
	ReplyToID      *string    `json:"reply_to_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	EditedAt       *time.Time `json:"edited_at,omitempty"`
	// DeletedAt is set when the sender deleted the message for everyone; the
	// body is blanked in that case so clients render a tombstone.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	// Reactions is filled by ListMessages (one batched query per page).
	Reactions []ReactionSummary `json:"reactions,omitempty"`
	// AttachmentIDs are media_files.id values; the client fetches a fresh
	// presigned URL per ID via GET /media/{id}/download-url rather than
	// this event carrying a URL directly, since presigned URLs expire and
	// this event may be delivered later via push+sync rather than live.
	AttachmentIDs []string `json:"attachment_ids,omitempty"`
	// Attachments carries display metadata (kind, mime, size, name) for the
	// same media, in the same order. No URL: clients resolve one per id via
	// GET /media/{id}/download-url because presigned URLs expire.
	Attachments []AttachmentInfo `json:"attachments,omitempty"`
	// Status ("sent" | "delivered" | "read") is filled by ListMessages for
	// the requester's own messages only, so history shows the same ticks the
	// live receipts produce.
	Status                 string  `json:"status,omitempty"`
	ForwardedFromMessageID *string `json:"forwarded_from_message_id,omitempty"`
}

// AttachmentInfo is the non-secret description of one attached file.
type AttachmentInfo struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	Name      string `json:"name,omitempty"`
}

// --- REST DTOs ---

type SendMessageRequest struct {
	ClientTmpID string `json:"client_tmp_id"`
	Type        string `json:"type"`
	Body        string `json:"body,omitempty"`
	ReplyToID   string `json:"reply_to_id,omitempty"`
	// AttachmentMediaIDs are media_files.id values previously obtained via
	// POST /media/presign-upload -> upload -> POST /media/{id}/confirm.
	// The sender must own each one (enforced by the media module when
	// attaching); order in this slice is preserved as display order.
	AttachmentMediaIDs []string `json:"attachment_media_ids,omitempty"`
	// ForwardedFromMessageID marks this as a forward of an existing
	// message. The server does not copy body/attachments from the
	// original — the client is expected to populate Body/AttachmentMediaIDs
	// itself (it already has the original message's content locally) — this
	// field is provenance for the "Forwarded" UI label only, same as most
	// real chat clients' forward semantics.
	ForwardedFromMessageID string `json:"forwarded_from_message_id,omitempty"`
}

type StartDirectConversationRequest struct {
	PeerUserID string `json:"peer_user_id"`
}

type ConversationSummary struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	PeerUserID  string   `json:"peer_user_id,omitempty"` // for type=direct only
	Name        *string  `json:"name,omitempty"`         // for type=group only (the group's name)
	LastMessage *Message `json:"last_message,omitempty"`
	UnreadCount int64    `json:"unread_count"`
	// Per-user list state (web client).
	PinnedAt   *time.Time `json:"pinned_at,omitempty"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	MutedUntil *time.Time `json:"muted_until,omitempty"`
}

type EditMessageRequest struct {
	Body string `json:"body"`
}

type MarkReadRequest struct {
	MessageID string `json:"message_id"`
}

type FlagRequest struct {
	Value bool `json:"value"`
}

type MessagePage struct {
	Messages   []Message `json:"messages"`
	NextCursor string    `json:"next_cursor,omitempty"`
}
