import type { Message } from "@/types/models";
import { serverBus } from "./serverBus";
import { db } from "./db";
import { uid } from "@/lib/utils";

/** Simulates the other side of a direct chat: delivered → read → typing → reply. Dev/mock only. */
const REPLIES = ["Got it 👍", "Sounds good!", "Haha, nice.", "Let me check and get back to you.", "Can we talk about it tomorrow?", "On my way!", "Perfect, thanks!"];

export function onUserSent(m: Message) {
  const s = db();
  const conv = s.conversations.find((c) => c.id === m.conversationId);
  if (!conv || conv.kind !== "direct" || !conv.peerId) return;
  const peer = s.users[conv.peerId];
  const t = (ms: number, fn: () => void) => setTimeout(fn, ms);

  t(500, () => {
    m.status = "delivered";
    serverBus.emit({ type: "message:delivered", conversationId: conv.id, messageId: m.id });
  });
  if (peer.presence !== "online") return;
  t(1400, () => {
    m.status = "read";
    serverBus.emit({ type: "message:read", conversationId: conv.id, userId: peer.id, upToMessageId: m.id });
  });
  t(2000, () => serverBus.emit({ type: "user:typing", conversationId: conv.id, userId: peer.id }));
  t(4200, () => {
    serverBus.emit({ type: "user:stop_typing", conversationId: conv.id, userId: peer.id });
    const reply: Message = {
      id: uid("m"), conversationId: conv.id, senderId: peer.id, text: REPLIES[Math.floor(Math.random() * REPLIES.length)],
      attachments: [], reactions: [], status: "delivered", createdAt: new Date().toISOString(),
    };
    s.messages[conv.id].push(reply);
    conv.lastMessage = { id: reply.id, text: reply.text, senderId: peer.id, createdAt: reply.createdAt, status: "delivered" };
    conv.updatedAt = reply.createdAt;
    serverBus.emit({ type: "message:new", message: { ...reply } });
  });
}
