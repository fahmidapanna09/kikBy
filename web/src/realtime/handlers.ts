import { handleCallEvent } from "@/calls/callClient";
import { navigate } from "@/lib/router";
import { notify } from "@/lib/nativeNotify";
import { session } from "@/services/session";
import { addNotification, loadConversations, loadFriends, removeConversation } from "@/state/actions";
import { store } from "@/state/store";
import { applyMessageDelete, applyMessageUpdate, applyReactions, applyStatus, receiveMessage, setPresence, setTyping } from "@/state/messages";
import type { Message } from "@/types/models";
import type { ServerEvent } from "./events";

/**
 * socket → event handler → app state → UI.
 * This file is the ONLY place that maps wire events to state transitions; sockets never touch the store
 * and components never touch sockets.
 */
/** System notification for a message from someone else, unless that chat is open on screen right now. */
function notifyMessage(m: Message): void {
  const st = store.get();
  if (m.senderId === session.get()?.user.id) return;
  if (st.activeConvId === m.conversationId && document.visibilityState === "visible") return;
  const conv = st.conversations.find((c) => c.id === m.conversationId);
  if (conv?.muted) return;
  const sender = st.users[m.senderId]?.displayName ?? "New message";
  const title = conv && conv.kind === "group" ? `${sender} · ${conv.title ?? "Group"}` : sender;
  const text = m.text || (m.attachments[0] ? "Sent an attachment" : "New message");
  void notify(title, text, `/chats/${m.conversationId}`);
}

const typingTimers = new Map<string, ReturnType<typeof setTimeout>>();

export function handleServerEvent(e: ServerEvent): void {
  switch (e.type) {
    case "message:new":
      receiveMessage(e.message);
      notifyMessage(e.message);
      setTyping(e.message.conversationId, e.message.senderId, false);
      break;
    case "message:update":
      applyMessageUpdate(e.message);
      break;
    case "message:delete":
      applyMessageDelete(e.conversationId, e.messageId, e.deletedAt);
      break;
    case "message:delivered":
      applyStatus(e.conversationId, "delivered", { messageId: e.messageId });
      break;
    case "message:read":
      applyStatus(e.conversationId, "read", { upTo: e.upToMessageId });
      break;
    case "message:reactions":
      applyReactions(e.conversationId, e.messageId, e.reactions);
      break;
    case "user:online":
      setPresence(e.userId, "online");
      break;
    case "user:offline":
      setPresence(e.userId, "offline", e.lastSeenAt);
      break;
    case "user:typing": {
      setTyping(e.conversationId, e.userId, true);
      const key = `${e.conversationId}:${e.userId}`;
      clearTimeout(typingTimers.get(key));
      // Safety net: if "stop_typing" is lost, clear after 6s.
      typingTimers.set(key, setTimeout(() => setTyping(e.conversationId, e.userId, false), 6000));
      break;
    }
    case "user:stop_typing":
      clearTimeout(typingTimers.get(`${e.conversationId}:${e.userId}`));
      setTyping(e.conversationId, e.userId, false);
      break;
    case "notification:new":
      addNotification(e.notification);
      if (e.notification.kind === "friend_request" || e.notification.kind === "friend_accept") void notify(e.notification.title || "KikBite", e.notification.body, "/friends");
      // Somebody sent / accepted a friend request: refresh the friends list so the new status shows immediately.
      if (e.notification.kind === "friend_request" || e.notification.kind === "friend_accept") void loadFriends({ silent: true });
      break;
    case "conversations:refresh":
      void loadConversations({ silent: true });
      window.dispatchEvent(new CustomEvent("kikbite:group-changed"));
      break;
    case "call:incoming":
      void notify("Incoming call", `${store.get().users[e.fromUserId]?.displayName ?? "Someone"} is calling you`, "/chats");
      handleCallEvent(e);
      break;
    case "call:joined":
    case "call:participant_joined":
    case "call:participant_left":
    case "call:signal":
    case "call:ended":
    case "call:dismissed":
    case "call:error":
      handleCallEvent(e);
      break;
    case "group:member_removed":
      if (e.userId === session.get()?.user.id) {
        const viewing = store.get().activeConvId === e.conversationId;
        removeConversation(e.conversationId);
        if (viewing) navigate("/chats");
      } else {
        void loadConversations({ silent: true });
        window.dispatchEvent(new CustomEvent("kikbite:group-changed", { detail: e.conversationId }));
      }
      break;
  }
}
