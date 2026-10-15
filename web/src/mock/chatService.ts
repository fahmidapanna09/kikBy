import type { ChatService } from "@/services/types";
import type { Conversation } from "@/types/models";
import { ServiceError } from "@/services/errors";
import { delay, readDelay, requireState } from "./util";

const sorted = (c: Conversation[]) =>
  [...c].sort((a, b) => Number(b.pinned) - Number(a.pinned) || +new Date(b.updatedAt) - +new Date(a.updatedAt));

export const chatService: ChatService = {
  async list() {
    await readDelay();
    return sorted(requireState().conversations).map((c) => ({ ...c }));
  },
  async get(id) {
    await readDelay();
    const c = requireState().conversations.find((x) => x.id === id);
    if (!c) throw new ServiceError("not_found", "This conversation doesn't exist or you no longer have access.", { status: 404 });
    return { ...c };
  },
  async startDirect(userId) {
    await delay(150, 300);
    const s = requireState();
    const peer = s.users[userId];
    if (!peer) throw new ServiceError("not_found", "User not found", { status: 404 });
    const existing = s.conversations.find((c) => c.kind === "direct" && c.peerId === userId);
    if (existing) return { ...existing };
    const conv: Conversation = {
      id: `c_${userId}`, kind: "direct", title: peer.displayName, memberIds: [s.meId, userId], peerId: userId,
      unreadCount: 0, pinned: false, muted: false, archived: false, updatedAt: new Date().toISOString(),
    };
    s.conversations.push(conv);
    s.messages[conv.id] = [];
    return { ...conv };
  },
  async update(id, patch) {
    await delay(80, 200);
    const c = requireState().conversations.find((x) => x.id === id);
    if (!c) throw new ServiceError("not_found", "Conversation not found", { status: 404 });
    Object.assign(c, patch);
    return { ...c };
  },
  async createGroup(name, memberIds) {
    await delay(150, 300);
    const s = requireState();
    const conv: Conversation = {
      id: `g_${Date.now().toString(36)}`, kind: "group", title: name.trim(), memberIds: [s.meId, ...memberIds],
      unreadCount: 0, pinned: false, muted: false, archived: false, updatedAt: new Date().toISOString(),
    };
    s.conversations.push(conv);
    s.messages[conv.id] = [];
    return { ...conv };
  },
  async members(id) {
    await readDelay();
    const c = requireState().conversations.find((x) => x.id === id);
    if (!c) throw new ServiceError("not_found", "Group not found", { status: 404 });
    return c.memberIds.map((userId, i) => ({ userId, role: i === 0 ? ("owner" as const) : ("member" as const) }));
  },
  async renameGroup(id, name) {
    await delay(80, 200);
    const c = requireState().conversations.find((x) => x.id === id);
    if (!c) throw new ServiceError("not_found", "Group not found", { status: 404 });
    c.title = name.trim();
    return { ...c };
  },
  async addMembers(id, userIds) {
    await delay(80, 200);
    const c = requireState().conversations.find((x) => x.id === id);
    if (!c) throw new ServiceError("not_found", "Group not found", { status: 404 });
    c.memberIds = [...new Set([...c.memberIds, ...userIds])];
  },
  async removeMember(id, userId) {
    await delay(80, 200);
    const s = requireState();
    const c = s.conversations.find((x) => x.id === id);
    if (!c) throw new ServiceError("not_found", "Group not found", { status: 404 });
    if (userId === s.meId) s.conversations = s.conversations.filter((x) => x.id !== id);
    else c.memberIds = c.memberIds.filter((m) => m !== userId);
  },
  async markRead(id) {
    await delay(40, 120);
    const c = requireState().conversations.find((x) => x.id === id);
    if (c) c.unreadCount = 0;
  },
};
