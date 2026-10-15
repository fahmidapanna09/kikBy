import type { IconName } from "@/icons/Icon";

export interface NavItem {
  to: string;
  label: string;
  icon: IconName;
  badge?: "chats" | "friends" | "notifications";
  /** path prefixes that mark this item active */
  match: string[];
}

export const NAV_ITEMS: NavItem[] = [
  { to: "/", label: "Home", icon: "home", match: ["/"] },
  { to: "/chats", label: "Chats", icon: "chat", badge: "chats", match: ["/chats"] },
  { to: "/friends", label: "Friends", icon: "users", badge: "friends", match: ["/friends"] },
  { to: "/notifications", label: "Notifications", icon: "bell", badge: "notifications", match: ["/notifications"] },
  { to: "/search", label: "Search", icon: "search", match: ["/search"] },
];

export const isActive = (item: NavItem, path: string) => (item.to === "/" ? path === "/" : item.match.some((m) => path === m || path.startsWith(`${m}/`)));
