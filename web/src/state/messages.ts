import type { Attachment, ID, Message, MessageStatus, Reaction } from "@/types/models";
import { services } from "@/services";
import { errorMessage } from "@/services/errors";
import { session } from "@/services/session";
import { attachmentLabel, uid } from "@/lib/utils";
import { store } from "./store";
import type { MessageThread } from "./store";
import { cacheUsers, loadConversations } from "./actions";

const PAGE = 30;
const meId = () => session.get()?.user.id ?? "";

const emptyThread = (): MessageThread => ({ items: [], status: "idle", hasMore: false, cursor: null, loadingMore: false });

function patchThread(convId: ID, fn: (t: MessageThread) => Partial<MessageThread>) {
  store.set((s) => {
    const t = s.threads[convId] ?? emptyThread();
    return { threads: { ...s.threads, [convId]: { ...t, ...fn(t) } } };
  });
}

function patchConversationLast(convId: ID) {
  store.set((s) => {
    const t = s.threads[convId];
    const last = t?.items[t.items.length - 1];
    if (!last) return {};
    return {
      conversations: s.conversations.map((c) =>
        c.id === convId
          ? { ...c, updatedAt: last.createdAt, lastMessage: { id: last.id, text: last.deletedAt ? "This message was deleted" : last.text || (last.attachments[0] ? attachmentLabel(last.attachments[0].kind) : ""), senderId: last.senderId, createdAt: last.createdAt, status: last.status } }
          : c,
      ),
    };
  });
}

/* ───────── Loading ───────── */

export async function openThread(convId: ID) {
  store.set({ activeConvId: convId });
  const existing = store.get().threads[convId];
  const conv = store.get().conversations.find((c) => c.id === convId);
  if (existing?.status === "ready") {
    markConversationRead(convId);
    return;
  }
  patchThread(convId, () => ({ status: "loading", error: undefined }));
  try {
    const page = await services.messages.list(convId, { limit: PAGE });
    // Divider: first of the trailing `unreadCount` messages that were not sent by me.
    let firstUnreadId: ID | undefined;
    const unread = conv?.unreadCount ?? 0;
    if (unread > 0) {
      const incoming = page.items.filter((m) => m.senderId !== meId());
      firstUnreadId = incoming[Math.max(0, incoming.length - unread)]?.id;
    }
    patchThread(convId, () => ({ items: page.items, status: "ready", hasMore: !!page.pagination.nextCursor, cursor: page.pagination.nextCursor ?? null, firstUnreadId }));
    markConversationRead(convId);
  } catch (e) {
    patchThread(convId, () => ({ status: "error", error: errorMessage(e, "Couldn't load messages.") }));
  }
}

export function closeThread(convId: ID) {
  if (store.get().activeConvId === convId) store.set({ activeConvId: null });
}

export async function loadOlder(convId: ID): Promise<number> {
  const t = store.get().threads[convId];
  if (!t || !t.hasMore || t.loadingMore || !t.cursor) return 0;
  patchThread(convId, () => ({ loadingMore: true }));
  try {
    const page = await services.messages.list(convId, { before: t.cursor, limit: PAGE });
    patchThread(convId, (cur) => ({ items: [...page.items, ...cur.items], hasMore: !!page.pagination.nextCursor, cursor: page.pagination.nextCursor ?? null, loadingMore: false }));
    return page.items.length;
  } catch {
    patchThread(convId, () => ({ loadingMore: false }));
    return 0;
  }
}

export function markConversationRead(convId: ID) {
  const c = store.get().conversations.find((x) => x.id === convId);
  if (!c || c.unreadCount === 0) return;
  store.set((s) => ({ conversations: s.conversations.map((x) => (x.id === convId ? { ...x, unreadCount: 0 } : x)) }));
  void services.chats.markRead(convId).catch(() => undefined);
}

/* ───────── Sending (optimistic) ───────── */

export function sendMessage(input: { conversationId: ID; text: string; replyToId?: ID; attachments?: Attachment[] }): string {
  const clientId = uid("c");
  const optimistic: Message = {
    id: clientId, clientId, conversationId: input.conversationId, senderId: meId(), text: input.text.trim(),
    attachments: input.attachments ?? [], replyToId: input.replyToId, reactions: [], status: "sending", createdAt: new Date().toISOString(),
  };
  patchThread(input.conversationId, (t) => ({ items: [...t.items, optimistic], firstUnreadId: undefined }));
  patchConversationLast(input.conversationId);
  void deliver(optimistic);
  return clientId;
}

async function deliver(m: Message) {
  try {
    const sent = await services.messages.send({ conversationId: m.conversationId, text: m.text, clientId: m.clientId as string, replyToId: m.replyToId, attachments: m.attachments });
    patchThread(m.conversationId, (t) => {
      // A fast server echo may already have added the real message; avoid duplicates.
      const hasReal = t.items.some((x) => x.id === sent.id);
      const items = hasReal ? t.items.filter((x) => x.clientId !== m.clientId || x.id === sent.id) : t.items.map((x) => (x.clientId === m.clientId ? { ...sent, status: x.status === "sending" ? sent.status : x.status } : x));
      return { items };
    });
    patchConversationLast(m.conversationId);
    const early = earlyReceipts.get(sent.id);
    if (early) {
      earlyReceipts.delete(sent.id);
      applyStatus(m.conversationId, early, early === "read" ? { upTo: sent.id } : { messageId: sent.id });
    }
  } catch (e) {
    setStatusByClientId(m.conversationId, m.clientId as string, "failed");
    console.warn("send failed:", errorMessage(e));
  }
}

function setStatusByClientId(convId: ID, clientId: string, status: MessageStatus) {
  patchThread(convId, (t) => ({ items: t.items.map((x) => (x.clientId === clientId ? { ...x, status } : x)) }));
}

export function retryMessage(m: Message) {
  if (m.status !== "failed") return;
  setStatusByClientId(m.conversationId, m.clientId ?? m.id, "sending");
  void deliver({ ...m, status: "sending" });
}

export function discardFailed(m: Message) {
  patchThread(m.conversationId, (t) => ({ items: t.items.filter((x) => x.id !== m.id) }));
  patchConversationLast(m.conversationId);
}

/* ───────── Mutations ───────── */

function replaceMessage(m: Message) {
  patchThread(m.conversationId, (t) => ({ items: t.items.map((x) => (x.id === m.id ? { ...x, ...m } : x)) }));
  patchConversationLast(m.conversationId);
}

/**
 * An edit changes text only. The live edit response/event doesn't repeat reactions or attachments, so keep what the
 * thread already has instead of letting the update blank them.
 */
function mergeEdit(m: Message) {
  patchThread(m.conversationId, (t) => ({
    items: t.items.map((x) => (x.id === m.id ? { ...x, ...m, reactions: x.reactions, attachments: x.attachments } : x)),
  }));
  patchConversationLast(m.conversationId);
}

export async function editMessage(m: Message, text: string) {
  const before = m;
  replaceMessage({ ...m, text, editedAt: new Date().toISOString() });
  try {
    mergeEdit(await services.messages.edit(m.id, text));
  } catch (e) {
    replaceMessage(before);
    throw e;
  }
}

export async function deleteMessage(m: Message) {
  const before = m;
  replaceMessage({ ...m, deletedAt: new Date().toISOString(), text: "", attachments: [], reactions: [] });
  try {
    replaceMessage(await services.messages.remove(m.id));
  } catch (e) {
    replaceMessage(before);
    throw e;
  }
}

export async function reactToMessage(m: Message, emoji: string) {
  const me = meId();
  const optimistic = (() => {
    const ex = m.reactions.find((r) => r.emoji === emoji);
    if (!ex) return [...m.reactions, { emoji, userIds: [me] }];
    const ids = ex.userIds.includes(me) ? ex.userIds.filter((u) => u !== me) : [...ex.userIds, me];
    return m.reactions.map((r) => (r.emoji === emoji ? { ...r, userIds: ids } : r)).filter((r) => r.userIds.length);
  })();
  replaceMessage({ ...m, reactions: optimistic });
  try {
    replaceMessage(await services.messages.react(m.id, emoji));
  } catch (e) {
    replaceMessage(m);
    throw e;
  }
}

export async function forwardMessage(m: Message, conversationIds: ID[]) {
  const sent = await services.messages.forward(m.id, conversationIds);
  for (const f of sent) {
    patchThread(f.conversationId, (t) => (t.status === "ready" ? { items: [...t.items, f] } : {}));
    patchConversationLast(f.conversationId);
  }
  await loadConversations({ silent: true });
}

/* ───────── Primitives used by the real-time layer ───────── */

export function receiveMessage(m: Message) {
  const s = store.get();
  const mine = m.senderId === meId();
  patchThread(m.conversationId, (t) => {
    if (t.items.some((x) => x.id === m.id)) return {};
    // Own message echoed by the server → reconcile with the optimistic copy.
    if (m.clientId && t.items.some((x) => x.clientId === m.clientId)) return { items: t.items.map((x) => (x.clientId === m.clientId ? m : x)) };
    return t.status === "ready" ? { items: [...t.items, m] } : {};
  });
  const known = s.conversations.some((c) => c.id === m.conversationId);
  if (!known) {
    void loadConversations({ silent: true });
    return;
  }
  const viewing = s.activeConvId === m.conversationId && document.visibilityState === "visible";
  store.set((st) => ({
    conversations: st.conversations.map((c) =>
      c.id === m.conversationId
        ? { ...c, updatedAt: m.createdAt, lastMessage: { id: m.id, text: m.text || (m.attachments[0] ? attachmentLabel(m.attachments[0].kind) : ""), senderId: m.senderId, createdAt: m.createdAt, status: m.status }, unreadCount: mine || viewing ? c.unreadCount : c.unreadCount + 1 }
        : c,
    ),
  }));
  if (viewing && !mine) markConversationRead(m.conversationId);
}

export function applyMessageUpdate(m: Message) {
  mergeEdit(m);
}

export function applyMessageDelete(convId: ID, messageId: ID, deletedAt: string) {
  patchThread(convId, (t) => ({ items: t.items.map((x) => (x.id === messageId ? { ...x, deletedAt, text: "", attachments: [], reactions: [] } : x)) }));
  patchConversationLast(convId);
}

export function applyReactions(convId: ID, messageId: ID, reactions: Reaction[]) {
  patchThread(convId, (t) => ({ items: t.items.map((x) => (x.id === messageId ? { ...x, reactions } : x)) }));
}

const RANK: Record<MessageStatus, number> = { failed: -1, sending: 0, sent: 1, delivered: 2, read: 3 };
/**
 * A receipt can outrun the sender's own HTTP response (the recipient acks over the socket while the POST is still
 * in flight, so the thread still holds the optimistic copy under its client id). Park it and apply it on reconcile.
 */
const earlyReceipts = new Map<ID, "delivered" | "read">();

export function applyStatus(convId: ID, status: "delivered" | "read", opts: { messageId?: ID; upTo?: ID }) {
  const ref = opts.messageId ?? opts.upTo;
  const known = ref ? (store.get().threads[convId]?.items.some((x) => x.id === ref) ?? false) : true;
  if (ref && !known) {
    if (RANK[earlyReceipts.get(ref) ?? "sent"] < RANK[status]) earlyReceipts.set(ref, status);
    if (earlyReceipts.size > 200) earlyReceipts.delete(earlyReceipts.keys().next().value as ID);
    return;
  }
  patchThread(convId, (t) => {
    const stop = opts.upTo ? t.items.findIndex((x) => x.id === opts.upTo) : -1;
    return {
      items: t.items.map((x, i) => {
        const match = opts.messageId ? x.id === opts.messageId : stop >= 0 ? i <= stop : false;
        return match && x.senderId === meId() && RANK[x.status] < RANK[status] ? { ...x, status } : x;
      }),
    };
  });
  patchConversationLast(convId);
}

export function setTyping(convId: ID, userId: ID, on: boolean) {
  store.set((s) => {
    const cur = s.typing[convId] ?? [];
    const next = on ? (cur.includes(userId) ? cur : [...cur, userId]) : cur.filter((u) => u !== userId);
    return { typing: { ...s.typing, [convId]: next } };
  });
}

export function setPresence(userId: ID, presence: "online" | "offline", lastSeenAt?: string) {
  store.set((s) => (s.users[userId] ? { users: { ...s.users, [userId]: { ...s.users[userId], presence, lastSeenAt: lastSeenAt ?? s.users[userId].lastSeenAt } } } : {}));
}

export { cacheUsers };
