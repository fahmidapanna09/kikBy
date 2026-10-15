import type { ClientEvent, ConnectionState, ServerEvent, SocketClient } from "./events";
import { serverBus } from "@/mock/serverBus";
import { db } from "@/mock/db";
import { uid } from "@/lib/utils";

/** In-memory SocketClient used in mock mode. Relays serverBus events + simulates presence and an occasional incoming message. */
export class MockSocket implements SocketClient {
  state: ConnectionState = "idle";
  private eventHandlers = new Set<(e: ServerEvent) => void>();
  private stateHandlers = new Set<(s: ConnectionState) => void>();
  private offBus: (() => void) | null = null;
  private timers: Array<ReturnType<typeof setTimeout> | ReturnType<typeof setInterval>> = [];

  private setState(s: ConnectionState) {
    this.state = s;
    this.stateHandlers.forEach((h) => h(s));
  }

  connect() {
    if (this.state === "open" || this.state === "connecting") return;
    this.setState("connecting");
    this.timers.push(
      setTimeout(() => {
        this.setState("open");
        this.offBus = serverBus.on((e) => this.eventHandlers.forEach((h) => h(e)));
        this.simulate();
      }, 350),
    );
  }

  disconnect() {
    this.timers.forEach((t) => clearTimeout(t as number));
    this.timers = [];
    this.offBus?.();
    this.offBus = null;
    if (this.state !== "idle") this.setState("closed");
  }

  send(_event: ClientEvent) {
    // A real server would persist read receipts / fan out typing. Nothing to do in the mock.
  }

  onEvent(h: (e: ServerEvent) => void) {
    this.eventHandlers.add(h);
    return () => this.eventHandlers.delete(h);
  }
  onState(h: (s: ConnectionState) => void) {
    this.stateHandlers.add(h);
    return () => this.stateHandlers.delete(h);
  }

  /** Presence flips + one unsolicited incoming message so notifications / unread badges can be seen live. */
  private simulate() {
    this.timers.push(
      setInterval(() => {
        const s = db();
        const people = Object.values(s.users).filter((u) => u.id !== s.meId);
        const u = people[Math.floor(Math.random() * people.length)];
        if (!u) return;
        const nowOnline = u.presence !== "online";
        u.presence = nowOnline ? "online" : "offline";
        if (!nowOnline) u.lastSeenAt = new Date().toISOString();
        serverBus.emit(nowOnline ? { type: "user:online", userId: u.id } : { type: "user:offline", userId: u.id, lastSeenAt: u.lastSeenAt as string });
      }, 25000),
    );
    this.timers.push(
      setTimeout(() => {
        const s = db();
        const conv = s.conversations.find((c) => c.id === "c_u_rafi");
        if (!conv) return;
        const m = { id: uid("m"), conversationId: conv.id, senderId: "u_rafi", text: "Btw, staging is up — take a look when you can 🚀", attachments: [], reactions: [], status: "delivered" as const, createdAt: new Date().toISOString() };
        s.messages[conv.id].push(m);
        conv.lastMessage = { id: m.id, text: m.text, senderId: m.senderId, createdAt: m.createdAt, status: m.status };
        conv.updatedAt = m.createdAt;
        serverBus.emit({ type: "user:typing", conversationId: conv.id, userId: "u_rafi" });
        setTimeout(() => {
          serverBus.emit({ type: "user:stop_typing", conversationId: conv.id, userId: "u_rafi" });
          serverBus.emit({ type: "message:new", message: m });
          const n = { id: uid("n"), kind: "message" as const, title: "Rafi Khan", body: m.text, actorId: "u_rafi", conversationId: conv.id, read: false, createdAt: m.createdAt };
          s.notifications.unshift(n);
          serverBus.emit({ type: "notification:new", notification: n });
        }, 1800);
      }, 20000),
    );
  }
}
