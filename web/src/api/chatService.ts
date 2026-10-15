import type { ChatService, MessageService } from "@/services/types";
import type { Conversation, ID, Message, User } from "@/types/models";
import { ServiceError } from "@/services/errors";
import { http } from "@/services/http";
import { session } from "@/services/session";
import { store } from "@/state/store";
import { uid } from "@/lib/utils";
import { liveUserService } from "./userService";
import { toConversation, toMessage, type ApiConversation, type ApiGroup, type ApiMessage } from "./chatMappers";

const enc = encodeURIComponent;
const meId = () => session.get()?.user.id ?? "";

/* ───────── profile cache (peer names for the chat list, sender names in groups) ───────── */

const profiles = new Map<ID, Promise<User | undefined>>();

function profile(id: ID): Promise<User | undefined> {
  let p = profiles.get(id);
  if (!p) {
    p = liveUserService.getProfile(id).catch(() => {
      profiles.delete(id); // allow a retry later
      return undefined;
    });
    profiles.set(id, p);
  }
  return p;
}

/** Fetch (once) and put users into the shared store cache so avatars/names resolve. */
export async function ensureUsers(ids: ID[]): Promise<void> {
  const need = [...new Set(ids)].filter((id) => id && !store.get().users[id]);
  if (!need.length) return;
  const got = (await Promise.all(need.map(profile))).filter((u): u is User => !!u);
  if (!got.length) return;
  store.set((s) => {
    const next = { ...s.users };
    for (const u of got) next[u.id] = { ...next[u.id], ...u };
    return { users: next };
  });
}

/* ───────── helpers over the already-loaded store ───────── */

function findMessage(id: ID): Message | undefined {
  for (const t of Object.values(store.get().threads)) {
    const m = t.items.find((x) => x.id === id);
    if (m) return m;
  }
  return undefined;
}

async function toLiveConversation(c: ApiConversation): Promise<Conversation> {
  const peer = c.peer_user_id ? await profile(c.peer_user_id) : undefined;
  if (peer) await ensureUsers([peer.id]);
  return toConversation(c, meId(), peer);
}

/* ───────── conversations ───────── */

export const liveChatService: ChatService = {
  async list() {
    const r = await http<{ conversations: ApiConversation[] }>("/conversations");
    const list = await Promise.all((r.conversations ?? []).map(toLiveConversation));
    return list.sort((a, b) => Number(b.pinned) - Number(a.pinned) || +new Date(b.updatedAt) - +new Date(a.updatedAt));
  },
  async get(id) {
    const found = (await liveChatService.list()).find((c) => c.id === id);
    if (!found) throw new ServiceError("not_found", "This conversation doesn't exist or you no longer have access.", { status: 404 });
    return found;
  },
  async startDirect(userId) {
    const r = await http<ApiConversation>("/conversations", { method: "POST", body: { peer_user_id: userId } });
    // the create response carries only id/type/peer; the list has last message + state
    const full = (await liveChatService.list()).find((c) => c.id === r.id);
    return full ?? (await toLiveConversation({ ...r, unread_count: 0 }));
  },
  async update(id, patch) {
    if (patch.pinned !== undefined) await http(`/conversations/${enc(id)}/pin`, { method: "PUT", body: { value: patch.pinned } });
    if (patch.archived !== undefined) await http(`/conversations/${enc(id)}/archive`, { method: "PUT", body: { value: patch.archived } });
    if (patch.muted !== undefined) {
      // "muted" has no duration in the UI → far-future timestamp; omitted `until` = unmute (backend contract)
      await http(`/conversations/${enc(id)}/mute`, { method: "PUT", body: patch.muted ? { until: new Date(Date.now() + 100 * 365 * 864e5).toISOString() } : {} });
    }
    const cur = store.get().conversations.find((c) => c.id === id);
    if (cur) return { ...cur, ...patch };
    return liveChatService.get(id);
  },
  async createGroup(name, memberIds) {
    const g = await http<ApiGroup>("/groups", { method: "POST", body: { name: name.trim(), member_user_ids: memberIds } });
    const ids = (g.members ?? []).map((m) => m.user_id);
    await ensureUsers(ids);
    return { id: g.conversation_id, kind: "group", title: g.name, memberIds: ids, unreadCount: 0, pinned: false, muted: false, archived: false, updatedAt: new Date().toISOString() };
  },
  async members(id) {
    const g = await http<ApiGroup>(`/groups/${enc(id)}`);
    const list = (g.members ?? []).map((m) => ({ userId: m.user_id, role: m.role }));
    await ensureUsers(list.map((m) => m.userId));
    return list;
  },
  async renameGroup(id, name) {
    await http(`/groups/${enc(id)}`, { method: "PATCH", body: { name: name.trim() } });
    const cur = store.get().conversations.find((c) => c.id === id);
    if (!cur) return liveChatService.get(id);
    return { ...cur, title: name.trim() };
  },
  async addMembers(id, userIds) {
    await http(`/groups/${enc(id)}/members`, { method: "POST", body: { user_ids: userIds } });
  },
  async removeMember(id, userId) {
    await http(`/groups/${enc(id)}/members/${enc(userId)}`, { method: "DELETE" });
  },
  async markRead(id) {
    const last = store.get().conversations.find((c) => c.id === id)?.lastMessage;
    if (!last) return;
    await http(`/conversations/${enc(id)}/read`, { method: "POST", body: { message_id: last.id } });
  },
};

/* ───────── messages ───────── */

async function postMessage(conversationId: ID, body: Record<string, unknown>): Promise<Message> {
  return toMessage(await http<ApiMessage>(`/conversations/${enc(conversationId)}/messages`, { method: "POST", body: { type: "text", ...body } }));
}

export const liveMessageService: MessageService = {
  async list(conversationId, opts = {}) {
    const q = opts.before ? `?cursor=${enc(opts.before)}` : "";
    const r = await http<{ messages: ApiMessage[]; next_cursor?: string }>(`/conversations/${enc(conversationId)}/messages${q}`);
    const items = (r.messages ?? []).map(toMessage).reverse(); // API is newest-first; the UI wants ascending
    void ensureUsers(items.map((m) => m.senderId));
    return { items, pagination: { page: 1, pageSize: items.length, total: items.length, nextCursor: r.next_cursor || null } };
  },
  async send({ conversationId, text, clientId, replyToId, attachments }) {
    const first = attachments?.[0];
    return postMessage(conversationId, {
      client_tmp_id: clientId,
      body: text,
      reply_to_id: replyToId,
      ...(first ? { type: first.kind === "file" ? "document" : first.kind, attachment_media_ids: attachments!.map((a) => a.id) } : {}),
    });
  },
  async edit(id, text) {
    return toMessage(await http<ApiMessage>(`/messages/${enc(id)}`, { method: "PATCH", body: { body: text } }));
  },
  async remove(id) {
    await http(`/messages/${enc(id)}?scope=everyone`, { method: "DELETE" });
    const m = findMessage(id);
    if (!m) throw new ServiceError("not_found", "Message not found");
    return { ...m, deletedAt: m.deletedAt ?? new Date().toISOString(), text: "", attachments: [], reactions: [] };
  },
  async react(id, emoji) {
    const m = findMessage(id);
    if (!m) throw new ServiceError("not_found", "Message not found");
    // The UI has already applied its optimistic toggle, so the current local state is the *target*:
    // I now hold `emoji` → set it on the server (one slot per user, replaces any other); I no longer do → clear it.
    const nowHolds = m.reactions.find((r) => r.emoji === emoji)?.userIds.includes(meId()) ?? false;
    if (nowHolds) await http(`/messages/${enc(id)}/reactions`, { method: "POST", body: { emoji } });
    else await http(`/messages/${enc(id)}/reactions`, { method: "DELETE" });
    // The authoritative reaction set arrives as a `message.reaction` socket event for every member (incl. me).
    return findMessage(id) ?? m;
  },
  async forward(id, conversationIds) {
    const src = findMessage(id);
    if (!src || src.deletedAt) throw new ServiceError("not_found", "Message not found");
    const out: Message[] = [];
    const first = src.attachments[0];
    // The server lets a member re-attach a file that is already in one of their conversations (no copy is made).
    const files = first ? { type: first.kind === "file" ? "document" : first.kind, attachment_media_ids: src.attachments.map((a) => a.id) } : {};
    for (const cid of conversationIds) out.push(await postMessage(cid, { client_tmp_id: uid("f"), body: src.text, forwarded_from_message_id: id, ...files }));
    return out;
  },
};
