package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/yourorg/chatplatform/internal/platform/middleware"
)

type WSHandler struct {
	svc      *Service
	hub      *Hub
	presence *Presence
	verifier middleware.TokenVerifier
	log      *zap.Logger
	upgrader websocket.Upgrader

	// allowedOrigins is the browser Origin allow-list (scheme://host[:port],
	// lower-cased). Requests without an Origin header (native clients such as
	// Android) are always accepted; a browser Origin must match this list or
	// the request's own Host.
	allowedOrigins map[string]struct{}
}

// wsSubprotocol is the fixed subprotocol the server selects; the browser also
// offers "bearer.<access token>" as a second entry, because the browser
// WebSocket API cannot set an Authorization header.
const wsSubprotocol = "kikbite.v1"

// SetAllowedOrigins configures the browser Origin allow-list.
func (h *WSHandler) SetAllowedOrigins(origins []string) {
	m := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		o = strings.ToLower(strings.TrimRight(strings.TrimSpace(o), "/"))
		if o != "" {
			m[o] = struct{}{}
		}
	}
	h.allowedOrigins = m
}

func (h *WSHandler) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // native client
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	_, ok := h.allowedOrigins[strings.ToLower(u.Scheme+"://"+u.Host)]
	return ok
}

// tokenFromRequest reads the access token from the Authorization header
// (native clients) or from the "bearer.<token>" subprotocol (browsers).
func tokenFromRequest(r *http.Request) string {
	const prefix = "Bearer "
	if a := r.Header.Get("Authorization"); len(a) > len(prefix) && a[:len(prefix)] == prefix {
		return a[len(prefix):]
	}
	for _, line := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, p := range strings.Split(line, ",") {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(p, "bearer.") && len(p) > len("bearer.") {
				return p[len("bearer."):]
			}
		}
	}
	return ""
}

func NewWSHandler(svc *Service, hub *Hub, presence *Presence, verifier middleware.TokenVerifier, log *zap.Logger) *WSHandler {
	h := &WSHandler{
		svc:      svc,
		hub:      hub,
		presence: presence,
		verifier: verifier,
		log:      log,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			Subprotocols:    []string{wsSubprotocol},
		},
	}
	h.upgrader.CheckOrigin = h.checkOrigin
	return h
}

// ServeWS authenticates via the standard Authorization: Bearer header (the
// Android client sets this on the handshake request itself, same as any
// other API call — no separate "ws ticket" endpoint needed for a native
// client that controls its own handshake headers).
func (h *WSHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	token := tokenFromRequest(r)
	if token == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	userID, deviceID, err := h.verifier.VerifyAccessToken(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	wsConn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("ws upgrade failed", zap.Error(err))
		return
	}

	c := &conn{
		ws:       wsConn,
		userID:   userID,
		deviceID: deviceID,
		send:     make(chan Envelope, outboundBuffer),
	}

	h.hub.Register(c)
	ctx := context.Background()
	_ = h.presence.MarkOnline(ctx, userID)
	h.broadcastPresence(ctx, userID, true)

	go h.writePump(c)
	h.readPump(ctx, c) // blocks until the connection closes

	h.hub.Unregister(c)
	h.dropCalls(ctx, userID)
	_ = h.presence.MarkOffline(ctx, userID)
	h.broadcastPresence(ctx, userID, false)
}

func (h *WSHandler) broadcastPresence(ctx context.Context, userID string, online bool) {
	// In a full implementation this fans out to the user's contacts only
	// (a contacts-list query), not globally — left as a Phase 3 (contacts
	// module) integration point. For now presence is available via the
	// conversation membership path (typing/read already scope to it).
}

func (h *WSHandler) readPump(ctx context.Context, c *conn) {
	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(pongWait))
		_ = h.presence.MarkOnline(ctx, c.userID)
		return nil
	})

	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			return // triggers cleanup in caller (ServeWS)
		}

		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			h.sendError(c, "invalid_envelope", "could not parse message")
			continue
		}
		h.handleInbound(ctx, c, env)
	}
}

func (h *WSHandler) handleInbound(ctx context.Context, c *conn, env Envelope) {
	switch env.Type {
	case EventMessageSend:
		var payload MessageSendPayload
		if err := decodePayload(env.Payload, &payload); err != nil {
			h.sendError(c, "invalid_payload", "malformed message.send payload")
			return
		}
		msg, err := h.svc.SendMessage(ctx, payload.ConversationID, c.userID, c.deviceID, SendMessageRequest{
			ClientTmpID:            payload.ClientTmpID,
			Type:                   payload.Type,
			Body:                   payload.Body,
			ReplyToID:              payload.ReplyToID,
			AttachmentMediaIDs:     payload.AttachmentMediaIDs,
			ForwardedFromMessageID: payload.ForwardedFromMessageID,
		})
		if err != nil {
			h.sendError(c, "send_failed", err.Error())
			return
		}
		ack := NewEnvelope(EventMessageAck, payload.ConversationID, c.deviceID, MessageAckPayload{
			ClientTmpID: payload.ClientTmpID, ServerID: msg.ID, Seq: msg.Seq,
		})
		select {
		case c.send <- ack:
		default:
		}

	case EventMessageDelivered:
		var payload DeliveredPayload
		if err := decodePayload(env.Payload, &payload); err != nil {
			return
		}
		_ = h.svc.MarkDelivered(ctx, env.ConversationID, payload.MessageID, c.userID)

	case EventMessageRead:
		var payload ReadPayload
		if err := decodePayload(env.Payload, &payload); err != nil {
			return
		}
		// Seq is not carried on the wire payload today; a real client also
		// sends the seq of the read message so MarkRead can bump
		// last_read_seq without an extra lookup. Left as a follow-up wire
		// format tweak; for now this only records the individual read receipt.
		_ = h.svc.MarkRead(ctx, env.ConversationID, payload.MessageID, c.userID, 0)

	case EventTypingStart:
		_ = h.svc.BroadcastTyping(ctx, env.ConversationID, c.userID, true)
	case EventTypingStop:
		_ = h.svc.BroadcastTyping(ctx, env.ConversationID, c.userID, false)

	case EventMessageReaction:
		var payload MessageReactionSetPayload
		if err := decodePayload(env.Payload, &payload); err != nil {
			h.sendError(c, "invalid_payload", "malformed message.reaction payload")
			return
		}
		var err error
		if payload.Emoji == "" {
			err = h.svc.RemoveReaction(ctx, c.userID, payload.MessageID)
		} else {
			err = h.svc.SetReaction(ctx, c.userID, payload.MessageID, payload.Emoji)
		}
		if err != nil {
			h.sendError(c, "reaction_failed", err.Error())
		}

	case EventCallStart, EventCallJoin, EventCallSignal, EventCallReject, EventCallLeave:
		h.handleCallEvent(ctx, c, env)

	default:
		h.sendError(c, "unknown_event_type", "unrecognized event type: "+env.Type)
	}
}

func (h *WSHandler) sendError(c *conn, code, message string) {
	env := NewEnvelope(EventError, "", c.deviceID, ErrorPayload{Code: code, Message: message})
	select {
	case c.send <- env:
	default:
	}
}

func (h *WSHandler) writePump(c *conn) {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()

	for {
		select {
		case env, ok := <-c.send:
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.ws.WriteJSON(env); err != nil {
				return
			}
		case <-ticker.C:
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func decodePayload(raw interface{}, out interface{}) error {
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
