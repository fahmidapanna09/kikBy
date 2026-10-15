import type { ClientEvent, ConnectionState, ServerEvent, SocketClient } from "./events";
import { config } from "@/services/config";
import { session } from "@/services/session";
import { decodeEnvelope, encodeClientEvent, type WireEnvelope } from "@/api/chatMappers";

/**
 * Production WebSocket transport against the Go backend's /ws (used when config.liveChat or mock === false).
 * Reconnects with exponential backoff + jitter and queues nothing while closed — the app re-syncs from REST on reconnect.
 *
 * Auth: browsers cannot set an Authorization header on a WebSocket, so the access token travels as a second
 * subprotocol entry ("kikbite.v1", "bearer.<token>") — never in the URL, so it stays out of proxy/access logs.
 * The server selects "kikbite.v1" and only accepts the handshake from an allow-listed Origin.
 * Frames are the backend's chat.Envelope; see api/chatMappers.ts for the mapping to the UI's event types.
 */
export class WebSocketClient implements SocketClient {
  state: ConnectionState = "idle";
  private ws: WebSocket | null = null;
  private attempts = 0;
  private wantOpen = false;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private eventHandlers = new Set<(e: ServerEvent) => void>();
  private stateHandlers = new Set<(s: ConnectionState) => void>();

  private setState(s: ConnectionState) {
    this.state = s;
    this.stateHandlers.forEach((h) => h(s));
  }

  connect() {
    this.wantOpen = true;
    this.open();
  }

  private open() {
    const tok = session.get()?.accessToken;
    if (!tok || this.ws) return;
    this.setState(this.attempts === 0 ? "connecting" : "reconnecting");
    const url = config.wsUrl.startsWith("ws") ? config.wsUrl : `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}${config.wsUrl}`;
    const ws = new WebSocket(url, ["kikbite.v1", `bearer.${tok}`]);
    this.ws = ws;
    ws.onopen = () => {
      this.attempts = 0;
      this.setState("open");
    };
    ws.onmessage = (m) => {
      try {
        const env = JSON.parse(String(m.data)) as WireEnvelope;
        const evt = env && typeof env.type === "string" ? decodeEnvelope(env) : null;
        if (evt) this.eventHandlers.forEach((h) => h(evt));
      } catch {
        /* ignore malformed frame */
      }
    };
    ws.onclose = () => {
      this.ws = null;
      if (!this.wantOpen) return this.setState("closed");
      this.attempts++;
      const delay = Math.min(30000, 500 * 2 ** this.attempts) * (0.7 + Math.random() * 0.6);
      this.setState("reconnecting");
      this.timer = setTimeout(() => this.open(), delay);
    };
    ws.onerror = () => ws.close();
  }

  disconnect() {
    this.wantOpen = false;
    if (this.timer) clearTimeout(this.timer);
    this.ws?.close();
    this.ws = null;
    this.setState("closed");
  }

  send(event: ClientEvent) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(encodeClientEvent(event));
  }
  onEvent(h: (e: ServerEvent) => void) {
    this.eventHandlers.add(h);
    return () => this.eventHandlers.delete(h);
  }
  onState(h: (s: ConnectionState) => void) {
    this.stateHandlers.add(h);
    return () => this.stateHandlers.delete(h);
  }
}
