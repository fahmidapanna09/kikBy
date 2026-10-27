import type { AppNotification, Attachment, AuthSession, Conversation, Friend, ID, Message, Pagination, Settings, User } from "@/types/models";

/** Service contracts. UI depends ONLY on these; mock/ and a future http implementation both satisfy them. */

export interface Page<T> {
  items: T[];
  pagination: Pagination;
}

export interface AuthService {
  login(input: { identifier: string; password: string; remember: boolean }): Promise<AuthSession>;
  register(input: { name: string; username: string; email: string; password: string; phone?: string }): Promise<AuthSession>;
  forgotPassword(email: string): Promise<void>;
  /** Set a new password using the 6-digit code e-mailed by forgotPassword. */
  resetPassword(input: { email: string; code: string; password: string }): Promise<void>;
  /** Confirm the e-mail address behind an emailed link (works without a session). */
  verifyEmail(code: string): Promise<void>;
  /** Re-send the verification e-mail for the signed-in user. */
  resendVerification(): Promise<{ alreadyVerified: boolean }>;
  logout(): Promise<void>;
}

export interface UserService {
  me(): Promise<User>;
  getProfile(id: ID): Promise<User>;
  updateProfile(
    patch: Partial<Pick<User, "displayName" | "username" | "bio" | "avatarUrl" | "email" | "phone">> & {
      /** Live only: uploaded image's media id to use as the profile picture; "" removes it; undefined leaves it unchanged. */
      avatarMediaId?: string;
    },
  ): Promise<User>;
  search(query: string): Promise<User[]>;
  /**
   * Change the sign-in e-mail. Live: needs the current password and only *requests* the change (a 6-digit confirmation code goes to the
   * new address; `pending: true`). Mock: applies immediately (`pending: false`).
   */
  changeEmail(input: { email: string; password?: string }): Promise<{ user: User; pending: boolean }>;
  getSettings(): Promise<Settings>;
  updateSettings(patch: Partial<Settings>): Promise<Settings>;
  changePassword(input: { current: string; next: string }): Promise<void>;
  deleteAccount(input: { password: string }): Promise<void>;
}

export interface ChatService {
  list(): Promise<Conversation[]>;
  get(id: ID): Promise<Conversation>;
  startDirect(userId: ID): Promise<Conversation>;
  update(id: ID, patch: Partial<Pick<Conversation, "pinned" | "muted" | "archived">>): Promise<Conversation>;
  markRead(id: ID): Promise<void>;
  createGroup(name: string, memberIds: ID[]): Promise<Conversation>;
  members(id: ID): Promise<GroupMember[]>;
  renameGroup(id: ID, name: string): Promise<Conversation>;
  addMembers(id: ID, userIds: ID[]): Promise<void>;
  /** Removing yourself = leaving the group. */
  removeMember(id: ID, userId: ID): Promise<void>;
}

export interface GroupMember {
  userId: ID;
  role: "owner" | "admin" | "member";
}

export interface MessageService {
  list(conversationId: ID, opts?: { before?: string; limit?: number }): Promise<Page<Message>>;
  send(input: { conversationId: ID; text: string; clientId: string; replyToId?: ID; attachments?: Attachment[] }): Promise<Message>;
  edit(id: ID, text: string): Promise<Message>;
  remove(id: ID): Promise<Message>;
  react(id: ID, emoji: string): Promise<Message>;
  forward(id: ID, conversationIds: ID[]): Promise<Message[]>;
}

export interface FriendService {
  list(): Promise<Friend[]>;
  sendRequest(userId: ID): Promise<Friend>;
  accept(userId: ID): Promise<Friend>;
  reject(userId: ID): Promise<void>;
  remove(userId: ID): Promise<void>;
  block(userId: ID): Promise<void>;
  unblock(userId: ID): Promise<void>;
}

export interface NotificationService {
  list(): Promise<AppNotification[]>;
  markRead(id: ID): Promise<void>;
  markAllRead(): Promise<void>;
}

export interface UploadService {
  upload(file: File, onProgress?: (pct: number) => void, signal?: AbortSignal, opts?: { voice?: boolean }): Promise<Attachment>;
  /**
   * Live mode only: attachments from the server carry no URL (signed links expire), so the UI asks for a fresh one
   * per attachment id. Mock attachments already have a local `url` and never call this.
   */
  resolveUrl?(attachmentId: string): Promise<string>;
  /** A small preview link when the server has one (`thumb: true`), else the full-size link. */
  resolveThumbnail?(attachmentId: string): Promise<{ url: string; thumb: boolean }>;
}

export interface Services {
  auth: AuthService;
  users: UserService;
  chats: ChatService;
  messages: MessageService;
  friends: FriendService;
  notifications: NotificationService;
  uploads: UploadService;
}
