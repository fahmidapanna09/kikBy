import type { MessageService } from "@/services/types";
import type { Message } from "@/types/models";
import { ServiceError } from "@/services/errors";
import { sanitizeText, uid } from "@/lib/utils";
import { delay, readDelay, requireState } from "./util";
import { onUserSent } from "./peerSim";

function find(id: string) {
  const s = requireState();
  for (const convId of Object.keys(s.messages)) {
    const m = s.messages[convId].find((x) => x.id === id);
    if (m) return { s, m, list: s.messages[convId] };
  }
  throw new ServiceError("not_found", "Message not found", { status: 404 });
}

function touchConversation(s: ReturnType<typeof requireState>, m: Message) {
  const c = s.conversations.find((x) => x.id === m.conversationId);
  if (!c) return;
  const last = s.messages[c.id][s.messages[c.id].length - 1];
  if (last) c.lastMessage = { id: last.id, text: last.deletedAt ? "This message was deleted" : last.text, senderId: last.senderId, createdAt: last.createdAt, status: last.status };
  c.updatedAt = last?.createdAt ?? c.updatedAt;
}

export const messageService: MessageService = {
  async list(conversationId, { before, limit = 30 } = {}) {
    await readDelay();
    const all = requireState().messages[conversationId];
    if (!all) throw new ServiceError("not_found", "Conversation not found", { status: 404 });
    const end = before ? all.findIndex((m) => m.id === before) : all.length;
    const stop = end < 0 ? all.length : end;
    const start = Math.max(0, stop - limit);
    return {
      items: all.slice(start, stop).map((m) => ({ ...m })),
      pagination: { page: 1, pageSize: limit, total: all.length, nextCursor: start > 0 ? all[start].id : null },
    };
  },

  async send({ conversationId, text, clientId, replyToId, attachments = [] }) {
    await delay(250, 600);
    const s = requireState();
    const clean = sanitizeText(text).trim();
    if (!clean && attachments.length === 0) throw new ServiceError("validation", "Message is empty", { status: 422 });
    if (clean.includes("/fail")) throw new ServiceError("send_failed", "Couldn't send (simulated failure). Tap retry.", { status: 503 });
    const list = s.messages[conversationId];
    if (!list) throw new ServiceError("not_found", "Conversation not found", { status: 404 });
    const m: Message = {
      id: uid("m"), clientId, conversationId, senderId: s.meId, text: clean, attachments, replyToId,
      reactions: [], status: "sent", createdAt: new Date().toISOString(),
    };
    list.push(m);
    touchConversation(s, m);
    onUserSent(m);
    return { ...m };
  },

  async edit(id, text) {
    await delay(150, 350);
    const { s, m } = find(id);
    if (m.senderId !== s.meId) throw new ServiceError("forbidden", "You can only edit your own messages", { status: 403 });
    if (m.deletedAt) throw new ServiceError("validation", "Deleted messages can't be edited", { status: 422 });
    const clean = sanitizeText(text).trim();
    if (!clean) throw new ServiceError("validation", "Message is empty", { status: 422 });
    m.text = clean;
    m.editedAt = new Date().toISOString();
    touchConversation(s, m);
    return { ...m };
  },

  async remove(id) {
    await delay(150, 300);
    const { s, m } = find(id);
    if (m.senderId !== s.meId) throw new ServiceError("forbidden", "You can only delete your own messages", { status: 403 });
    m.deletedAt = new Date().toISOString();
    m.text = "";
    m.attachments = [];
    m.reactions = [];
    touchConversation(s, m);
    return { ...m };
  },

  async react(id, emoji) {
    await delay(60, 160);
    const { s, m } = find(id);
    const existing = m.reactions.find((r) => r.emoji === emoji);
    if (existing) {
      existing.userIds = existing.userIds.includes(s.meId) ? existing.userIds.filter((u) => u !== s.meId) : [...existing.userIds, s.meId];
      m.reactions = m.reactions.filter((r) => r.userIds.length > 0);
    } else m.reactions = [...m.reactions, { emoji, userIds: [s.meId] }];
    return { ...m, reactions: m.reactions.map((r) => ({ ...r })) };
  },

  async forward(id, conversationIds) {
    await delay(250, 500);
    const { s, m } = find(id);
    const out: Message[] = [];
    for (const cid of conversationIds) {
      const list = s.messages[cid];
      if (!list) continue;
      const copy: Message = {
        id: uid("m"), conversationId: cid, senderId: s.meId, text: m.text, attachments: m.attachments, forwardedFrom: m.senderId,
        reactions: [], status: "sent", createdAt: new Date().toISOString(),
      };
      list.push(copy);
      touchConversation(s, copy);
      out.push({ ...copy });
    }
    return out;
  },
};
