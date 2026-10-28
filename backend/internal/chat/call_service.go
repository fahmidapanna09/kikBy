package chat

import (
	"context"
	"errors"
	"sync"
	"time"
)

// callStore is the persistence the call logic needs; *Repository implements it (call_repository.go).
type callStore interface {
	IsMember(ctx context.Context, conversationID, userID string) (bool, error)
	GetConversationType(ctx context.Context, conversationID string) (string, error)
	ActiveMemberIDs(ctx context.Context, conversationID string) ([]string, error)
	ExpireStaleCalls(ctx context.Context, conversationID string) error
	LiveCall(ctx context.Context, conversationID string) (Call, bool, error)
	CreateCall(ctx context.Context, conversationID, initiatorID, kind string) (Call, error) // ErrCallLive when one is already live
	GetCall(ctx context.Context, callID string) (Call, error)
	JoinCall(ctx context.Context, callID, userID string, max int) (Call, error)
	LeaveCall(ctx context.Context, callID, userID string) (remaining int, wasJoined bool, err error)
	// EndCall ends a live call (only when its status is requireStatus, if given) and marks everybody as left. changed=false when it was not live / did not match.
	EndCall(ctx context.Context, callID, status, reason, requireStatus string) (call Call, changed bool, err error)
	JoinedUsers(ctx context.Context, callID string) ([]string, error)
	CallsFor(ctx context.Context, conversationID string, limit int) ([]Call, error)
	LiveCallIDsOfUser(ctx context.Context, userID string) ([]string, error)
}

// ErrCallLive is returned by CreateCall when the conversation already has a live call.
var ErrCallLive = errors.New("chat: conversation already has a live call")

type callPublisher interface {
	Publish(ctx context.Context, userID string, env Envelope) error
}

// CallService runs the call state machine. Media is peer-to-peer (WebRTC mesh); this only keeps call state and relays
// signaling. All durable state is in the database, so any API node can serve any call event.
type CallService struct {
	store       callStore
	pub         callPublisher
	ice         IceConfig
	blocked     func(ctx context.Context, conversationID, userID string) (bool, error)
	now         func() time.Time
	ringTimeout time.Duration
	// push (optional) is told about every person a call rings, so an offline phone can get a push notification.
	push func(ctx context.Context, toUserID, fromUserID, conversationID string)

	mu     sync.Mutex
	timers map[string]*time.Timer
}

func NewCallService(store callStore, pub callPublisher, ice IceConfig, blocked func(ctx context.Context, conversationID, userID string) (bool, error)) *CallService {
	return &CallService{store: store, pub: pub, ice: ice, blocked: blocked, now: time.Now, ringTimeout: ringTimeout, timers: map[string]*time.Timer{}}
}

func (s *CallService) send(ctx context.Context, userID, eventType, conversationID string, payload interface{}) {
	_ = s.pub.Publish(ctx, userID, NewEnvelope(eventType, conversationID, "", payload))
}

// Start begins a call in a conversation, or joins the live one if there already is one.
func (s *CallService) Start(ctx context.Context, conversationID, userID, kind string) (Call, error) {
	if kind != CallVoice && kind != CallVideo {
		return Call{}, ErrCallInvalid
	}
	ok, err := s.store.IsMember(ctx, conversationID, userID)
	if err != nil {
		return Call{}, err
	}
	if !ok {
		return Call{}, ErrNotAMember
	}
	if s.blocked != nil {
		if b, err := s.blocked(ctx, conversationID, userID); err != nil {
			return Call{}, err
		} else if b {
			return Call{}, ErrCallBlocked
		}
	}
	if err := s.store.ExpireStaleCalls(ctx, conversationID); err != nil {
		return Call{}, err
	}
	if live, found, err := s.store.LiveCall(ctx, conversationID); err != nil {
		return Call{}, err
	} else if found {
		return s.Join(ctx, live.ID, userID)
	}
	call, err := s.store.CreateCall(ctx, conversationID, userID, kind)
	if errors.Is(err, ErrCallLive) { // lost a race with another starter
		if live, found, e2 := s.store.LiveCall(ctx, conversationID); e2 == nil && found {
			return s.Join(ctx, live.ID, userID)
		}
	}
	if err != nil {
		return Call{}, err
	}
	members, err := s.store.ActiveMemberIDs(ctx, conversationID)
	if err != nil {
		return Call{}, err
	}
	for _, m := range members {
		if m != userID {
			s.send(ctx, m, EventCallIncoming, conversationID, CallIncomingPayload{Call: call, FromUserID: userID})
			if s.push != nil {
				s.push(ctx, m, userID, conversationID)
			}
		}
	}
	s.send(ctx, userID, EventCallJoined, conversationID, CallJoinedPayload{Call: call, Participants: []string{}, IceServers: s.ice.ServersFor(userID, s.now())})
	s.armRingTimer(call.ID)
	return call, nil
}

// Join answers a ringing call or joins a group call that is in progress.
func (s *CallService) Join(ctx context.Context, callID, userID string) (Call, error) {
	call, err := s.store.GetCall(ctx, callID)
	if err != nil {
		return Call{}, err
	}
	ok, err := s.store.IsMember(ctx, call.ConversationID, userID)
	if err != nil {
		return Call{}, err
	}
	if !ok {
		return Call{}, ErrNotAMember
	}
	if !call.Live() {
		return Call{}, ErrCallEnded
	}
	call, err = s.store.JoinCall(ctx, callID, userID, maxCallParticipants)
	if err != nil {
		return Call{}, err
	}
	// Read who is in the call AFTER committing our own join: two people joining at the same moment then both see each other
	// (the web client resolves the resulting double offer with a deterministic "polite peer" rule).
	joined, err := s.store.JoinedUsers(ctx, callID)
	if err != nil {
		return Call{}, err
	}
	var before []string
	for _, u := range joined {
		if u != userID {
			before = append(before, u)
		}
	}
	if before == nil {
		before = []string{}
	}
	s.cancelRingTimer(callID, call)
	s.send(ctx, userID, EventCallJoined, call.ConversationID, CallJoinedPayload{Call: call, Participants: before, IceServers: s.ice.ServersFor(userID, s.now())})
	for _, u := range before {
		s.send(ctx, u, EventCallParticipantJoined, call.ConversationID, CallParticipantPayload{CallID: callID, UserID: userID})
	}
	return call, nil
}

// Leave is "hang up". A 1:1 call ends for both sides when either leaves; a group call ends when the last person leaves.
func (s *CallService) Leave(ctx context.Context, callID, userID string) error {
	call, err := s.store.GetCall(ctx, callID)
	if err != nil {
		return err
	}
	if !call.Live() {
		return nil
	}
	remaining, wasJoined, err := s.store.LeaveCall(ctx, callID, userID)
	if err != nil {
		return err
	}
	if !wasJoined {
		return nil
	}
	convType, err := s.store.GetConversationType(ctx, call.ConversationID)
	if err != nil {
		return err
	}
	if remaining == 0 || convType == "direct" {
		status := CallEnded
		if call.AnsweredAt == nil {
			status = CallCancelled
		}
		return s.finish(ctx, callID, status, "hangup", "")
	}
	joined, err := s.store.JoinedUsers(ctx, callID)
	if err != nil {
		return err
	}
	for _, u := range joined {
		s.send(ctx, u, EventCallParticipantLeft, call.ConversationID, CallParticipantPayload{CallID: callID, UserID: userID})
	}
	return nil
}

// Reject declines a ring. In a 1:1 call that ends the call; in a group it only stops ringing on the rejecting user's devices.
func (s *CallService) Reject(ctx context.Context, callID, userID string) error {
	call, err := s.store.GetCall(ctx, callID)
	if err != nil {
		return err
	}
	ok, err := s.store.IsMember(ctx, call.ConversationID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotAMember
	}
	if !call.Live() {
		return nil
	}
	joined, err := s.store.JoinedUsers(ctx, callID)
	if err != nil {
		return err
	}
	for _, u := range joined {
		if u == userID { // already in the call: rejecting is hanging up
			return s.Leave(ctx, callID, userID)
		}
	}
	convType, err := s.store.GetConversationType(ctx, call.ConversationID)
	if err != nil {
		return err
	}
	if convType == "direct" {
		return s.finish(ctx, callID, CallDeclined, "declined", "")
	}
	s.send(ctx, userID, EventCallDismissed, call.ConversationID, CallDismissedPayload{CallID: callID})
	return nil
}

// Signal relays one WebRTC offer/answer/ICE message between two people who are both in the call.
func (s *CallService) Signal(ctx context.Context, callID, fromUserID, toUserID string, data interface{}) error {
	if toUserID == "" || toUserID == fromUserID || data == nil {
		return ErrCallInvalid
	}
	call, err := s.store.GetCall(ctx, callID)
	if err != nil {
		return err
	}
	if !call.Live() {
		return ErrCallEnded
	}
	joined, err := s.store.JoinedUsers(ctx, callID)
	if err != nil {
		return err
	}
	var haveFrom, haveTo bool
	for _, u := range joined {
		haveFrom = haveFrom || u == fromUserID
		haveTo = haveTo || u == toUserID
	}
	if !haveFrom || !haveTo {
		return ErrCallInvalid
	}
	s.send(ctx, toUserID, EventCallSignal, call.ConversationID, CallSignalPayload{CallID: callID, FromUserID: fromUserID, Data: data})
	return nil
}

// History returns the most recent calls of a conversation the caller belongs to (live one first when present).
func (s *CallService) History(ctx context.Context, conversationID, userID string, limit int) ([]Call, error) {
	ok, err := s.store.IsMember(ctx, conversationID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotAMember
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	_ = s.store.ExpireStaleCalls(ctx, conversationID)
	return s.store.CallsFor(ctx, conversationID, limit)
}

// DropUser removes a user from every live call; used when their last connection closes.
func (s *CallService) DropUser(ctx context.Context, userID string) {
	ids, err := s.store.LiveCallIDsOfUser(ctx, userID)
	if err != nil {
		return
	}
	for _, id := range ids {
		_ = s.Leave(ctx, id, userID)
	}
}

// finish ends a live call and tells every member of the conversation.
func (s *CallService) finish(ctx context.Context, callID, status, reason, requireStatus string) error {
	call, changed, err := s.store.EndCall(ctx, callID, status, reason, requireStatus)
	if err != nil || !changed {
		return err
	}
	s.mu.Lock()
	if t, ok := s.timers[callID]; ok {
		t.Stop()
		delete(s.timers, callID)
	}
	s.mu.Unlock()
	members, err := s.store.ActiveMemberIDs(ctx, call.ConversationID)
	if err != nil {
		return err
	}
	for _, m := range members {
		s.send(ctx, m, EventCallEnded, call.ConversationID, CallEndedPayload{Call: call})
	}
	return nil
}

func (s *CallService) armRingTimer(callID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timers[callID] = time.AfterFunc(s.ringTimeout, func() { s.onRingTimeout(callID) })
}

func (s *CallService) cancelRingTimer(callID string, call Call) {
	if call.Status != CallActive {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.timers[callID]; ok {
		t.Stop()
		delete(s.timers, callID)
	}
}

func (s *CallService) onRingTimeout(callID string) {
	s.mu.Lock()
	delete(s.timers, callID)
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	call, err := s.store.GetCall(ctx, callID)
	if err != nil || call.Status != CallRinging {
		return
	}
	_ = s.finish(ctx, callID, CallMissed, "timeout", CallRinging)
}
