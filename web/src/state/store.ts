import type { AppNotification, Conversation, Friend, ID, Message, Settings, User } from "@/types/models";
import type { ConnectionState } from "@/realtime/events";
import { createStore, useStore } from "./createStore";

export type LoadStatus = "idle" | "loading" | "ready" | "error";

export interface MessageThread {
  items: Message[]; // ascending by time
  status: LoadStatus;
  error?: string;
  hasMore: boolean;
  cursor: string | null;
  loadingMore: boolean;
  /** id of first unread message when the thread was opened — drives the "New messages" divider */
  firstUnreadId?: ID;
}

export interface AppState {
  ready: boolean; // session bootstrapped
  settings: Settings | null;
  convStatus: LoadStatus;
  convError?: string;
  conversations: Conversation[];
  threads: Record<ID, MessageThread>;
  users: Record<ID, User>; // cache of every user we've seen (presence kept live here)
  friendsStatus: LoadStatus;
  friendsError?: string;
  friends: Friend[];
  notifStatus: LoadStatus;
  notifError?: string;
  notifications: AppNotification[];
  typing: Record<ID, ID[]>; // conversationId -> userIds currently typing
  connection: ConnectionState;
  online: boolean; // browser network state
  activeConvId: ID | null; // conversation currently open (drives auto-read)
}

export const initialState = (): AppState => ({
  ready: false,
  settings: null,
  convStatus: "idle",
  conversations: [],
  threads: {},
  users: {},
  friendsStatus: "idle",
  friends: [],
  notifStatus: "idle",
  notifications: [],
  typing: {},
  connection: "idle",
  online: typeof navigator === "undefined" ? true : navigator.onLine,
  activeConvId: null,
});

export const store = createStore<AppState>(initialState);
export const useApp = <S,>(selector: (s: AppState) => S): S => useStore(store, selector);
