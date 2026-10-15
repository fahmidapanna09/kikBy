package chat

import (
	"errors"
	"time"
)

// Call signaling events. Client -> server: start, join, signal, reject, leave.
// Server -> client: incoming, joined, participant_joined, participant_left, signal, ended, dismissed.
const (
	EventCallStart             = "call.start"              // client -> server {kind}
	EventCallJoin              = "call.join"               // client -> server {call_id}   (answer a ring / join a group call)
	EventCallSignal            = "call.signal"             // both ways {call_id, to_user_id|from_user_id, data}
	EventCallReject            = "call.reject"             // client -> server {call_id}
	EventCallLeave             = "call.leave"              // client -> server {call_id}   (hang up)
	EventCallIncoming          = "call.incoming"           // server -> other members
	EventCallJoined            = "call.joined"             // server -> the user that started/joined: {call, participants, ice_servers}
	EventCallParticipantJoined = "call.participant_joined" // server -> people already in the call
	EventCallParticipantLeft   = "call.participant_left"
	EventCallEnded             = "call.ended"     // server -> all conversation members
	EventCallDismissed         = "call.dismissed" // server -> a user's other devices: stop ringing
)

const (
	CallVoice = "voice"
	CallVideo = "video"

	CallRinging   = "ringing"
	CallActive    = "active"
	CallEnded     = "ended"
	CallMissed    = "missed"
	CallDeclined  = "declined"
	CallCancelled = "cancelled"

	// maxCallParticipants caps a mesh call: every extra person adds an upload stream for everybody.
	maxCallParticipants = 4
	// ringTimeout is how long an unanswered call rings before it is recorded as missed.
	ringTimeout = 45 * time.Second
)

var (
	ErrCallNotFound = errors.New("chat: call not found")
	ErrCallEnded    = errors.New("chat: this call has already ended")
	ErrCallFull     = errors.New("chat: this call is full")
	ErrCallBlocked  = errors.New("chat: calling is not allowed between these users")
	ErrCallInvalid  = errors.New("chat: invalid call request")
	ErrCallDisabled = errors.New("chat: calls are not enabled")
)

// Call is the wire + storage shape of one call.
type Call struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id"`
	InitiatorID    string     `json:"initiator_id"`
	Kind           string     `json:"kind"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	AnsweredAt     *time.Time `json:"answered_at,omitempty"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	EndReason      string     `json:"end_reason,omitempty"`
}

func (c Call) Live() bool { return c.Status == CallRinging || c.Status == CallActive }

// IceServer is one entry of RTCConfiguration.iceServers.
type IceServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type CallStartPayload struct {
	Kind string `json:"kind"`
}

type CallRefPayload struct {
	CallID string `json:"call_id"`
}

type CallSignalPayload struct {
	CallID     string      `json:"call_id"`
	ToUserID   string      `json:"to_user_id,omitempty"`
	FromUserID string      `json:"from_user_id,omitempty"`
	Data       interface{} `json:"data"` // opaque to the server: {type:"offer"|"answer", sdp} or {type:"candidate", candidate}
}

type CallIncomingPayload struct {
	Call       Call   `json:"call"`
	FromUserID string `json:"from_user_id"`
}

type CallJoinedPayload struct {
	Call         Call        `json:"call"`
	Participants []string    `json:"participants"` // everyone already in the call, excluding the joiner
	IceServers   []IceServer `json:"ice_servers"`
}

type CallParticipantPayload struct {
	CallID string `json:"call_id"`
	UserID string `json:"user_id"`
}

type CallEndedPayload struct {
	Call Call `json:"call"`
}

type CallDismissedPayload struct {
	CallID string `json:"call_id"`
}
