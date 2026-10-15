import { ensureUsers } from "@/api/chatService";
import { config } from "@/services/config";
import { session } from "@/services/session";
import { loadConversations, loadFriends } from "@/state/actions";
import { clearPresence } from "@/state/presenceMemo";
import { store } from "@/state/store";
import type { ClientEvent, SocketClient } from "./events";
import { handleServerEvent } from "./handlers";
import { MockSocket } from "./mockSocket";
import { WebSocketClient } from "./webSocketClient";

let socket: SocketClient | null = null;
let offs: Array<() => void> = [];

/** Start the app's single real-time connection. Safe to call repeatedly. */
export function startRealtime() {
  if (socket) return;
  const live = !config.mock || config.liveChat;
  socket = live ? new WebSocketClient() : new MockSocket();
  let wasOpen = false;
  offs = [
    socket.onEvent((e) => {
      handleServerEvent(e);
      // Tell the sender it reached this device ("delivered" tick). Only the live socket understands this event.
      if (live && e.type === "message:new" && e.message.senderId !== session.get()?.user.id) {
        socket?.send({ type: "message:delivered", conversationId: e.message.conversationId, messageId: e.message.id });
        void ensureUsers([e.message.senderId]);
      }
    }),
    socket.onState((s) => {
      store.set({ connection: s });
      // After a reconnect, anything missed while offline is re-synced from REST.
      if (s === "open") {
        clearPresence(); // the server re-announces everyone who is online on each new connection
        const me = session.get()?.user.id;
        store.set((st) => ({ users: Object.fromEntries(Object.entries(st.users).map(([id, u]) => [id, id === me || u.presence === "offline" ? u : { ...u, presence: "offline" as const }])) }));
        if (wasOpen) {
          void loadConversations({ silent: true });
          void loadFriends({ silent: true });
        }
        wasOpen = true;
      }
    }),
  ];
  socket.connect();
  if (live || config.liveAuth) {
    // Coming back to the app (phone unlocked, tab re-focused): pick up friend changes made while it was in the background.
    const onVisible = () => {
      if (document.visibilityState === "visible") void loadFriends({ silent: true });
    };
    document.addEventListener("visibilitychange", onVisible);
    offs.push(() => document.removeEventListener("visibilitychange", onVisible));
  }
}

export function stopRealtime() {
  offs.forEach((o) => o());
  offs = [];
  socket?.disconnect();
  socket = null;
}

/** Components talk to the transport only through this function. */
export function sendRealtime(e: ClientEvent) {
  socket?.send(e);
}
