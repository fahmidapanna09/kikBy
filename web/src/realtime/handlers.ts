import { handleCallEvent } from "@/calls/callClient";
import { navigate } from "@/lib/router";
import { session } from "@/services/session";
import { addNotification, loadConversations, removeConversation } from "@/state/actions";
import { store } from "@/state/store";
import { applyMessageDelete, applyMessageUpdate, applyReactions, applyStatus, receiveMessage, setPresence, setTyping } from "@/state/messages";
import type { ServerEvent } from "./events";

/**
 * socket → event handler → app state → UI.
 * This file is the ONLY place that maps wire events to state transitions; sockets never touch the store
 * and components never touch sockets.
 */
const typingTimers = new Map<string, ReturnType<typeof setTimeout>>();

export function handleServerEvent(e: ServerEvent): void {
  switch (e.type) {
    case "message:new":
      receiveMessage(e.message);
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
      break;
    case "conversations:refresh":
      void loadConversations({ silent: true });
      window.dispatchEvent(new CustomEvent("kikbite:group-changed"));
      break;
    case "call:incoming":
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
