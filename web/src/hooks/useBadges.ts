import { useApp } from "@/state/store";

/** Derived counters. Each selector returns a number → referentially stable. */
export function useBadges() {
  const chats = useApp((s) => s.conversations.reduce((n, c) => n + (c.muted || c.archived ? 0 : c.unreadCount), 0));
  const friends = useApp((s) => s.friends.reduce((n, f) => n + (f.status === "incoming" ? 1 : 0), 0));
  const notifications = useApp((s) => s.notifications.reduce((n, x) => n + (x.read ? 0 : 1), 0));
  return { chats, friends, notifications };
}
