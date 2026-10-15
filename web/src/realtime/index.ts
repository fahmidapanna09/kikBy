import { ensureUsers } from "@/api/chatService";
import { config } from "@/services/config";
import { session } from "@/services/session";
import { loadConversations } from "@/state/actions";
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
      if (s === "open" && wasOpen) void loadConversations({ silent: true });
      if (s === "open") wasOpen = true;
    }),
  ];
  socket.connect();
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
