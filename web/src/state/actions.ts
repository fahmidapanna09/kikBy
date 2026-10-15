import type { AppNotification, Conversation, Friend, ID, User } from "@/types/models";
import { services } from "@/services";
import { errorMessage } from "@/services/errors";
import { store } from "./store";

/** Imperative state transitions. UI calls these; they call services; they write the store. */

export function cacheUsers(users: User[]) {
  store.set((s) => {
    const next = { ...s.users };
    for (const u of users) next[u.id] = { ...next[u.id], ...u };
    return { users: next };
  });
}

export async function loadConversations(opts: { silent?: boolean } = {}) {
  if (!opts.silent) store.set({ convStatus: "loading", convError: undefined });
  try {
    const list = await services.chats.list();
    store.set({ conversations: list, convStatus: "ready" });
  } catch (e) {
    store.set({ convStatus: "error", convError: errorMessage(e, "Couldn't load your chats.") });
  }
}

export async function loadFriends() {
  store.set({ friendsStatus: "loading", friendsError: undefined });
  try {
    const list = await services.friends.list();
    cacheUsers(list.map((f) => f.user));
    store.set({ friends: list, friendsStatus: "ready" });
  } catch (e) {
    store.set({ friendsStatus: "error", friendsError: errorMessage(e, "Couldn't load your friends.") });
  }
}

export async function loadNotifications() {
  store.set({ notifStatus: "loading", notifError: undefined });
  try {
    const list = await services.notifications.list();
    store.set({ notifications: list, notifStatus: "ready" });
  } catch (e) {
    store.set({ notifStatus: "error", notifError: errorMessage(e, "Couldn't load notifications.") });
  }
}

export function upsertConversation(c: Conversation) {
  store.set((s) => {
    const exists = s.conversations.some((x) => x.id === c.id);
    return { conversations: exists ? s.conversations.map((x) => (x.id === c.id ? c : x)) : [c, ...s.conversations] };
  });
}

export function removeConversation(id: ID) {
  store.set((s) => ({ conversations: s.conversations.filter((c) => c.id !== id), activeConvId: s.activeConvId === id ? null : s.activeConvId }));
}

export function addNotification(n: AppNotification) {
  store.set((s) => ({ notifications: [n, ...s.notifications.filter((x) => x.id !== n.id)] }));
}

export function setFriend(f: Friend) {
  store.set((s) => ({ friends: s.friends.some((x) => x.user.id === f.user.id) ? s.friends.map((x) => (x.user.id === f.user.id ? f : x)) : [...s.friends, f] }));
}

export function removeFriendLocal(userId: ID) {
  store.set((s) => ({ friends: s.friends.filter((x) => x.user.id !== userId) }));
}

/** Called once after login / app start. */
export async function bootstrap(meId: ID) {
  const me = (await services.users.me().catch(() => null)) as User | null;
  if (me) cacheUsers([me]);
  const settings = await services.users.getSettings().catch(() => null);
  store.set({ settings, ready: true });
  void meId;
  await Promise.all([loadConversations(), loadFriends(), loadNotifications()]);
}

export function resetApp() {
  store.reset();
}
