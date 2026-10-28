package chat

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingInterval   = 25 * time.Second // must be < pongWait
	maxMessageSize = 64 * 1024        // 64KB per WS frame; large media goes through the media/S3 flow, not WS
	outboundBuffer = 64               // bounded per-connection outbound queue (see docs: backpressure handling)
)

func userChannel(userID string) string { return "chat:user:" + userID }

// conn wraps one authenticated WebSocket connection (one device/session).
type conn struct {
	ws        *websocket.Conn
	userID    string
	deviceID  string
	send      chan Envelope
	closeOnce sync.Once
}

// Hub tracks local connections on this node and uses Redis pub/sub so that a
// message for a user connected to a *different* node still reaches them.
// Every send goes through Redis (even for same-node delivery) to keep the
// delivery path uniform — see docs section 18, "cross-node fan-out".
type Hub struct {
	rdb *redis.Client
	log *zap.Logger

	mu          sync.RWMutex
	connsByUser map[string]map[*conn]struct{} // local connections only
	subCancel   map[string]context.CancelFunc // one redis subscription per user with >=1 local conn
}

func NewHub(rdb *redis.Client, log *zap.Logger) *Hub {
	return &Hub{
		rdb:         rdb,
		log:         log,
		connsByUser: make(map[string]map[*conn]struct{}),
		subCancel:   make(map[string]context.CancelFunc),
	}
}

func (h *Hub) Register(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	set, ok := h.connsByUser[c.userID]
	if !ok {
		set = make(map[*conn]struct{})
		h.connsByUser[c.userID] = set
	}
	set[c] = struct{}{}

	if _, subscribed := h.subCancel[c.userID]; !subscribed {
		ctx, cancel := context.WithCancel(context.Background())
		h.subCancel[c.userID] = cancel
		go h.subscribeLoop(ctx, c.userID)
	}
}

// HasLocalConn reports whether the user still has a WebSocket open on this node.
func (h *Hub) HasLocalConn(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connsByUser[userID]) > 0
}

func (h *Hub) Unregister(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	set, ok := h.connsByUser[c.userID]
	if !ok {
		return
	}
	delete(set, c)
	c.closeOnce.Do(func() { close(c.send) })

	if len(set) == 0 {
		delete(h.connsByUser, c.userID)
		if cancel, ok := h.subCancel[c.userID]; ok {
			cancel()
			delete(h.subCancel, c.userID)
		}
	}
}

// subscribeLoop is started once per user (the first time any of their
// devices connects on this node) and forwards every event published to
// their Redis channel to all of that user's local connections.
func (h *Hub) subscribeLoop(ctx context.Context, userID string) {
	pubsub := h.rdb.Subscribe(ctx, userChannel(userID))
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var env Envelope
			if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
				h.log.Error("hub: failed to unmarshal envelope from redis", zap.Error(err))
				continue
			}
			h.deliverLocal(userID, env)
		}
	}
}

func (h *Hub) deliverLocal(userID string, env Envelope) {
	h.mu.RLock()
	conns := h.connsByUser[userID]
	targets := make([]*conn, 0, len(conns))
	for c := range conns {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.send <- env:
		default:
			// Outbound queue full: drop presence/typing (non-critical) but
			// never drop message.* events — those recover via push + sync
			// instead of being silently lost, per the backpressure policy.
			if env.Type == EventTypingStart || env.Type == EventTypingStop || env.Type == EventPresenceUpdate {
				h.log.Warn("hub: dropping non-critical event, outbound queue full", zap.String("user_id", userID), zap.String("type", env.Type))
			} else {
				h.log.Warn("hub: outbound queue full for critical event, connection will be closed to force resync", zap.String("user_id", userID), zap.String("type", env.Type))
				c.closeOnce.Do(func() { c.ws.Close() })
			}
		}
	}
}

// Publish sends an event to a user's channel; delivery to their connected
// devices (on this node or any other) happens via subscribeLoop above.
func (h *Hub) Publish(ctx context.Context, userID string, env Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return h.rdb.Publish(ctx, userChannel(userID), data).Err()
}

// SubscriberCount is how many nodes currently hold a live socket for the user (0 = nobody is connected, so a push
// is the only way to reach them). On a Redis error it answers 0: better one extra push than a missed one.
func (h *Hub) SubscriberCount(ctx context.Context, userID string) int64 {
	m, err := h.rdb.PubSubNumSub(ctx, userChannel(userID)).Result()
	if err != nil {
		return 0
	}
	return m[userChannel(userID)]
}

// PublishEvent is a convenience wrapper for callers outside this package
// (e.g. stories.Service) that just want to push a typed event to a user
// without building an Envelope themselves — satisfies stories.EventPublisher
// structurally, same module-boundary approach as AttachmentLinker/BlockChecker.
func (h *Hub) PublishEvent(ctx context.Context, userID, eventType string, payload any) error {
	return h.Publish(ctx, userID, NewEnvelope(eventType, "", "", payload))
}

// IsUserOnlineLocally is a cheap local-only check; the authoritative
// "is this user online anywhere" check would query a Redis presence set
// maintained alongside this hub (set on Register/Unregister) — see
// SetPresence/IsOnline in presence.go.
func (h *Hub) IsUserOnlineLocally(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns, ok := h.connsByUser[userID]
	return ok && len(conns) > 0
}
