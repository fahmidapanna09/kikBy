package chat

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- in-memory fakes ---

type fakeCallStore struct {
	mu       sync.Mutex
	members  map[string]map[string]bool // conv -> user -> active
	convType map[string]string
	calls    map[string]*Call
	parts    map[string]map[string]*time.Time // call -> user -> left_at (nil = joined)
	seq      int
}

func newFakeCallStore() *fakeCallStore {
	return &fakeCallStore{members: map[string]map[string]bool{}, convType: map[string]string{}, calls: map[string]*Call{}, parts: map[string]map[string]*time.Time{}}
}

func (f *fakeCallStore) addConv(id, typ string, users ...string) {
	f.members[id] = map[string]bool{}
	for _, u := range users {
		f.members[id][u] = true
	}
	f.convType[id] = typ
}

func (f *fakeCallStore) IsMember(_ context.Context, c, u string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.members[c][u], nil
}
func (f *fakeCallStore) GetConversationType(_ context.Context, c string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.convType[c], nil
}
func (f *fakeCallStore) ActiveMemberIDs(_ context.Context, c string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for u := range f.members[c] {
		out = append(out, u)
	}
	return out, nil
}
func (f *fakeCallStore) ExpireStaleCalls(context.Context, string) error { return nil }
func (f *fakeCallStore) LiveCall(_ context.Context, c string) (Call, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.calls {
		if call.ConversationID == c && call.Live() {
			return *call, true, nil
		}
	}
	return Call{}, false, nil
}
func (f *fakeCallStore) CreateCall(_ context.Context, c, init, kind string) (Call, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.calls {
		if call.ConversationID == c && call.Live() {
			return Call{}, ErrCallLive
		}
	}
	f.seq++
	id := "call" + string(rune('0'+f.seq))
	call := &Call{ID: id, ConversationID: c, InitiatorID: init, Kind: kind, Status: CallRinging, StartedAt: time.Now()}
	f.calls[id] = call
	f.parts[id] = map[string]*time.Time{init: nil}
	return *call, nil
}
func (f *fakeCallStore) GetCall(_ context.Context, id string) (Call, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.calls[id]
	if !ok {
		return Call{}, ErrCallNotFound
	}
	return *c, nil
}
func (f *fakeCallStore) joinedLocked(id string) []string {
	var out []string
	for u, left := range f.parts[id] {
		if left == nil {
			out = append(out, u)
		}
	}
	return out
}
func (f *fakeCallStore) JoinCall(_ context.Context, id, u string, max int) (Call, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls[id]
	if !c.Live() {
		return Call{}, ErrCallEnded
	}
	others := 0
	for _, j := range f.joinedLocked(id) {
		if j != u {
			others++
		}
	}
	if others+1 > max {
		return Call{}, ErrCallFull
	}
	f.parts[id][u] = nil
	if c.Status == CallRinging && u != c.InitiatorID {
		now := time.Now()
		c.Status, c.AnsweredAt = CallActive, &now
	}
	return *c, nil
}
func (f *fakeCallStore) LeaveCall(_ context.Context, id, u string) (int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	was := false
	if left, ok := f.parts[id][u]; ok && left == nil {
		now := time.Now()
		f.parts[id][u] = &now
		was = true
	}
	return len(f.joinedLocked(id)), was, nil
}
func (f *fakeCallStore) EndCall(_ context.Context, id, status, reason, require string) (Call, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls[id]
	if !c.Live() || (require != "" && c.Status != require) {
		return *c, false, nil
	}
	now := time.Now()
	c.Status, c.EndReason, c.EndedAt = status, reason, &now
	for u := range f.parts[id] {
		f.parts[id][u] = &now
	}
	return *c, true, nil
}
func (f *fakeCallStore) JoinedUsers(_ context.Context, id string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.joinedLocked(id), nil
}
func (f *fakeCallStore) CallsFor(context.Context, string, int) ([]Call, error) { return nil, nil }
func (f *fakeCallStore) LiveCallIDsOfUser(_ context.Context, u string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for id, parts := range f.parts {
		if left, ok := parts[u]; ok && left == nil && f.calls[id].Live() {
			out = append(out, id)
		}
	}
	return out, nil
}

type sentEnv struct {
	to  string
	env Envelope
}
type fakePub struct {
	mu   sync.Mutex
	sent []sentEnv
}

func (p *fakePub) Publish(_ context.Context, u string, env Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, sentEnv{u, env})
	return nil
}
func (p *fakePub) to(u, typ string) []Envelope {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []Envelope
	for _, s := range p.sent {
		if s.to == u && s.env.Type == typ {
			out = append(out, s.env)
		}
	}
	return out
}

func newCallTest(t *testing.T) (*CallService, *fakeCallStore, *fakePub) {
	t.Helper()
	st := newFakeCallStore()
	st.addConv("dm", "direct", "A", "B")
	st.addConv("grp", "group", "A", "B", "C", "D", "E")
	pub := &fakePub{}
	return NewCallService(st, pub, IceConfig{StunURLs: []string{"stun:s"}}, nil), st, pub
}

var bg = context.Background()

func TestDirectCallLifecycle(t *testing.T) {
	s, _, pub := newCallTest(t)
	call, err := s.Start(bg, "dm", "A", CallVideo)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub.to("B", EventCallIncoming)) != 1 || len(pub.to("A", EventCallJoined)) != 1 || len(pub.to("A", EventCallIncoming)) != 0 {
		t.Fatalf("start must ring B and confirm to A only: %+v", pub.sent)
	}
	joined := pub.to("A", EventCallJoined)[0].Payload.(CallJoinedPayload)
	if len(joined.Participants) != 0 || len(joined.IceServers) != 1 {
		t.Fatalf("bad joined payload %+v", joined)
	}
	if _, err := s.Join(bg, call.ID, "B"); err != nil {
		t.Fatal(err)
	}
	pj := pub.to("B", EventCallJoined)[0].Payload.(CallJoinedPayload)
	if pj.Call.Status != CallActive || len(pj.Participants) != 1 || pj.Participants[0] != "A" {
		t.Fatalf("joiner must see A and an active call: %+v", pj)
	}
	if len(pub.to("A", EventCallParticipantJoined)) != 1 {
		t.Fatal("A must be told B joined")
	}
	// signaling relay
	if err := s.Signal(bg, call.ID, "B", "A", map[string]string{"type": "offer"}); err != nil {
		t.Fatal(err)
	}
	sig := pub.to("A", EventCallSignal)
	if len(sig) != 1 || sig[0].Payload.(CallSignalPayload).FromUserID != "B" {
		t.Fatalf("signal not relayed with sender stamped by the server: %+v", sig)
	}
	if err := s.Signal(bg, call.ID, "C", "A", map[string]string{}); !errors.Is(err, ErrCallInvalid) {
		t.Fatalf("outsider signal must be refused, got %v", err)
	}
	if err := s.Signal(bg, call.ID, "B", "B", map[string]string{}); !errors.Is(err, ErrCallInvalid) {
		t.Fatal("self signal must be refused")
	}
	// either side hanging up ends a 1:1 call for both
	if err := s.Leave(bg, call.ID, "A"); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"A", "B"} {
		ev := pub.to(u, EventCallEnded)
		if len(ev) != 1 || ev[0].Payload.(CallEndedPayload).Call.Status != CallEnded {
			t.Fatalf("%s must get call.ended(ended): %+v", u, ev)
		}
	}
	if err := s.Signal(bg, call.ID, "A", "B", map[string]string{}); !errors.Is(err, ErrCallEnded) {
		t.Fatalf("signal after end must fail, got %v", err)
	}
	if _, err := s.Join(bg, call.ID, "B"); !errors.Is(err, ErrCallEnded) {
		t.Fatalf("join after end must fail, got %v", err)
	}
}

func TestDirectRejectAndCancel(t *testing.T) {
	s, _, pub := newCallTest(t)
	c, _ := s.Start(bg, "dm", "A", CallVoice)
	if err := s.Reject(bg, c.ID, "B"); err != nil {
		t.Fatal(err)
	}
	if ev := pub.to("A", EventCallEnded); len(ev) != 1 || ev[0].Payload.(CallEndedPayload).Call.Status != CallDeclined {
		t.Fatalf("caller must learn the call was declined: %+v", ev)
	}
	pub.sent = nil
	c2, _ := s.Start(bg, "dm", "A", CallVoice)
	if c2.ID == c.ID {
		t.Fatal("a new call must be created after the old one ended")
	}
	if err := s.Leave(bg, c2.ID, "A"); err != nil {
		t.Fatal(err)
	}
	if ev := pub.to("B", EventCallEnded); len(ev) != 1 || ev[0].Payload.(CallEndedPayload).Call.Status != CallCancelled {
		t.Fatalf("callee must see a cancelled call: %+v", ev)
	}
}

func TestRingTimeoutMissedButNeverKillsActiveCall(t *testing.T) {
	s, _, pub := newCallTest(t)
	s.ringTimeout = 30 * time.Millisecond
	_, err := s.Start(bg, "dm", "A", CallVoice)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if ev := pub.to("B", EventCallEnded); len(ev) != 1 || ev[0].Payload.(CallEndedPayload).Call.Status != CallMissed {
		t.Fatalf("unanswered call must end as missed for B: %+v", ev)
	}
	if ev := pub.to("A", EventCallEnded); len(ev) != 1 {
		t.Fatal("caller must be told too")
	}
	// answered in time: the timer must not end it
	pub.sent = nil
	c, _ := s.Start(bg, "dm", "A", CallVoice)
	if _, err := s.Join(bg, c.ID, "B"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if len(pub.to("A", EventCallEnded)) != 0 {
		t.Fatal("an answered call must survive the ring timeout")
	}
}

func TestGroupCall(t *testing.T) {
	s, _, pub := newCallTest(t)
	c, err := s.Start(bg, "grp", "A", CallVideo)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"B", "C", "D", "E"} {
		if len(pub.to(u, EventCallIncoming)) != 1 {
			t.Fatalf("%s must be rung", u)
		}
	}
	// a second "start" while live joins the same call
	again, err := s.Start(bg, "grp", "B", CallVoice)
	if err != nil || again.ID != c.ID {
		t.Fatalf("start during a live call must join it: %v %v", again.ID, err)
	}
	for _, u := range []string{"C", "D"} {
		if _, err := s.Join(bg, c.ID, u); err != nil {
			t.Fatalf("%s: %v", u, err)
		}
	}
	if _, err := s.Join(bg, c.ID, "E"); !errors.Is(err, ErrCallFull) {
		t.Fatalf("5th participant must be refused (cap %d), got %v", maxCallParticipants, err)
	}
	last := pub.to("D", EventCallJoined)[0].Payload.(CallJoinedPayload)
	if len(last.Participants) != 3 {
		t.Fatalf("4th joiner must be told about the 3 others: %+v", last.Participants)
	}
	if len(pub.to("A", EventCallParticipantJoined)) != 3 {
		t.Fatal("A must see B, C and D join")
	}
	if err := s.Leave(bg, c.ID, "B"); err != nil {
		t.Fatal(err)
	}
	if len(pub.to("A", EventCallParticipantLeft)) != 1 || len(pub.to("E", EventCallEnded)) != 0 {
		t.Fatal("one person leaving must not end a group call")
	}
	// rejoin after leaving
	if _, err := s.Join(bg, c.ID, "B"); err != nil {
		t.Fatalf("rejoin: %v", err)
	}
	// E rejecting only dismisses E's own ring
	if err := s.Reject(bg, c.ID, "E"); err != nil {
		t.Fatal(err)
	}
	if len(pub.to("E", EventCallDismissed)) != 1 || len(pub.to("A", EventCallEnded)) != 0 {
		t.Fatal("group reject must only dismiss for the rejecting user")
	}
	for _, u := range []string{"A", "B", "C", "D"} {
		if err := s.Leave(bg, c.ID, u); err != nil {
			t.Fatal(err)
		}
	}
	if ev := pub.to("E", EventCallEnded); len(ev) != 1 || ev[0].Payload.(CallEndedPayload).Call.Status != CallEnded {
		t.Fatalf("the last person leaving must end the call for every member: %+v", ev)
	}
}

func TestStartGuards(t *testing.T) {
	s, st, _ := newCallTest(t)
	if _, err := s.Start(bg, "dm", "Z", CallVoice); !errors.Is(err, ErrNotAMember) {
		t.Fatalf("non-member: %v", err)
	}
	if _, err := s.Start(bg, "dm", "A", "hologram"); !errors.Is(err, ErrCallInvalid) {
		t.Fatalf("bad kind: %v", err)
	}
	s.blocked = func(context.Context, string, string) (bool, error) { return true, nil }
	if _, err := s.Start(bg, "dm", "A", CallVoice); !errors.Is(err, ErrCallBlocked) {
		t.Fatalf("blocked: %v", err)
	}
	s.blocked = nil
	c, _ := s.Start(bg, "dm", "A", CallVoice)
	st.members["dm"]["X"] = true
	if _, err := s.Join(bg, c.ID, "Z"); !errors.Is(err, ErrNotAMember) {
		t.Fatalf("outsider join: %v", err)
	}
}

func TestDropUserLeavesLiveCalls(t *testing.T) {
	s, _, pub := newCallTest(t)
	c, _ := s.Start(bg, "dm", "A", CallVoice)
	_, _ = s.Join(bg, c.ID, "B")
	s.DropUser(bg, "B") // B's last connection closed
	if len(pub.to("A", EventCallEnded)) != 1 {
		t.Fatal("dropping one side of a 1:1 call must end it for the other")
	}
}

func TestIceServers(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	only := IceConfig{StunURLs: []string{"stun:a"}}.ServersFor("u1", now)
	if len(only) != 1 || only[0].Username != "" {
		t.Fatalf("stun only: %+v", only)
	}
	cfg := IceConfig{StunURLs: []string{"stun:a"}, TurnURLs: []string{"turn:t:3478"}, TurnSecret: "s3cret", TTL: time.Hour}
	got := cfg.ServersFor("u1", now)
	if len(got) != 2 {
		t.Fatalf("want stun+turn: %+v", got)
	}
	wantUser := "1700003600:u1"
	mac := hmac.New(sha1.New, []byte("s3cret"))
	mac.Write([]byte(wantUser))
	if got[1].Username != wantUser || got[1].Credential != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("coturn REST credential mismatch: %+v", got[1])
	}
	static := IceConfig{TurnURLs: []string{"turn:x"}, TurnUsername: "me", TurnCredential: "pw"}.ServersFor("u1", now)
	if len(static) != 1 || static[0].Username != "me" || static[0].Credential != "pw" {
		t.Fatalf("static creds: %+v", static)
	}
	if got := (IceConfig{TurnURLs: []string{"turn:x"}}).ServersFor("u", now); len(got) != 0 {
		t.Fatalf("TURN url without credentials must not be advertised: %+v", got)
	}
	if !strings.HasPrefix(IceConfigFromEnv().StunURLs[0], "stun:") {
		t.Fatal("default STUN missing")
	}
}
