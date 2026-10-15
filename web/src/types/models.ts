// Typed domain models shared by services, mock data, state and UI.
// Shapes mirror what the Go backend returns (ISO-8601 strings for times, string ids).

export type ID = string;
export type ISODate = string;

export type Presence = "online" | "offline" | "away";

export interface User {
  id: ID;
  username: string;
  displayName: string;
  email?: string;
  /** undefined = unknown (mock mode); false shows the "verify your e-mail" banner. Not enforced at login. */
  emailVerified?: boolean;
  /** A new address waiting for its confirmation code; the login e-mail stays `email` until then. */
  pendingEmail?: string;
  /** Optional, unverified contact number (no SMS/OTP check yet). */
  phone?: string;
  avatarUrl?: string;
  /** index into the 6-colour avatar palette, derived from id when absent */
  avatarTone?: 1 | 2 | 3 | 4 | 5 | 6;
  bio?: string;
  presence: Presence;
  lastSeenAt?: ISODate;
  createdAt: ISODate;
}

export interface AuthSession {
  user: User;
  accessToken: string;
  refreshToken: string;
  /** epoch ms when the access token expires */
  expiresAt: number;
  remember: boolean;
}

export type AttachmentKind = "image" | "video" | "audio" | "file";

export interface Attachment {
  id: ID;
  kind: AttachmentKind;
  name: string;
  url: string;
  mimeType: string;
  sizeBytes: number;
  width?: number;
  height?: number;
}

export type MessageStatus = "sending" | "sent" | "delivered" | "read" | "failed";

export interface Reaction {
  emoji: string;
  userIds: ID[];
}

export interface Message {
  id: ID;
  /** client-generated id so optimistic sends can be reconciled with the server echo */
  clientId?: string;
  conversationId: ID;
  senderId: ID;
  text: string;
  attachments: Attachment[];
  replyToId?: ID;
  forwardedFrom?: ID;
  reactions: Reaction[];
  status: MessageStatus;
  createdAt: ISODate;
  editedAt?: ISODate;
  deletedAt?: ISODate;
}

export type ConversationKind = "direct" | "group";

export interface Conversation {
  id: ID;
  kind: ConversationKind;
  title: string;
  avatarUrl?: string;
  avatarTone?: 1 | 2 | 3 | 4 | 5 | 6;
  memberIds: ID[];
  /** for direct chats: the other participant */
  peerId?: ID;
  lastMessage?: Pick<Message, "id" | "text" | "senderId" | "createdAt" | "status">;
  unreadCount: number;
  pinned: boolean;
  muted: boolean;
  archived: boolean;
  updatedAt: ISODate;
}

export type FriendStatus = "friends" | "incoming" | "outgoing" | "blocked";

export interface Friend {
  user: User;
  status: FriendStatus;
  since?: ISODate;
  mutualCount?: number;
}

export type NotificationKind = "message" | "friend_request" | "friend_accept" | "mention" | "system";

export interface AppNotification {
  id: ID;
  kind: NotificationKind;
  title: string;
  body: string;
  actorId?: ID;
  conversationId?: ID;
  read: boolean;
  createdAt: ISODate;
}

export interface Pagination {
  page: number;
  pageSize: number;
  total: number;
  nextCursor?: string | null;
}

export interface ApiError {
  code: string;
  message: string;
  /** field -> message, for form validation errors returned by the server */
  fields?: Record<string, string>;
  status?: number;
}

export type ApiResponse<T> = { ok: true; data: T; pagination?: Pagination } | { ok: false; error: ApiError };

/** Per-user preferences — one object per settings tab. */
export interface Settings {
  privacy: {
    lastSeen: "everyone" | "friends" | "nobody";
    readReceipts: boolean;
    typingIndicator: boolean;
    profilePhoto: "everyone" | "friends" | "nobody";
  };
  notifications: { messages: boolean; groups: boolean; friendRequests: boolean; sound: boolean; preview: boolean };
  appearance: { density: "comfortable" | "compact"; fontScale: 90 | 100 | 110 | 120; reduceMotion: boolean };
  chat: { enterToSend: boolean; mediaAutoDownload: boolean; spellcheck: boolean };
}
