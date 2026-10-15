import type { ID, User } from "@/types/models";

/**
 * Who is online right now, as told by the socket. Kept outside the user cache on purpose: REST responses
 * (friends list, search, profiles) carry no presence and used to overwrite the live value with "offline",
 * and a presence event can arrive before the user has been cached.
 */
const memo = new Map<ID, { presence: "online" | "offline"; lastSeenAt?: string }>();

export function rememberPresence(userId: ID, presence: "online" | "offline", lastSeenAt?: string) {
  memo.set(userId, { presence, lastSeenAt: lastSeenAt ?? memo.get(userId)?.lastSeenAt });
}

/** Forget everything (a fresh socket connection re-announces who is online). */
export function clearPresence() {
  memo.clear();
}

/** Overlay the live presence on a user coming from REST. Unknown users are shown offline. */
export function withLivePresence<T extends User>(u: T): T {
  const m = memo.get(u.id);
  return m ? { ...u, presence: m.presence, lastSeenAt: m.lastSeenAt ?? u.lastSeenAt } : u;
}
