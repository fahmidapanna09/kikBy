import type { AppNotification, Friend, User } from "@/types/models";
import { config } from "@/services/config";

/** Public, cache-friendly link to a user's profile picture (the server redirects to a short-lived signed URL). `v` busts the cache when it changes. */
export const avatarLink = (userId: string, mediaId?: string) => `${config.apiBase}/users/${encodeURIComponent(userId)}/avatar${mediaId ? `?v=${encodeURIComponent(mediaId)}` : ""}`;

/** GET /v1/users/me (backend users.MeResponse, snake_case) → web User. */
export interface ApiMe {
  user_id: string;
  username?: string;
  email?: string;
  email_verified?: boolean;
  pending_email?: string;
  phone_number?: string;
  contact_phone?: string;
  display_name?: string;
  about?: string;
  avatar_media_id?: string;
}

export function toUser(m: ApiMe, createdAt = new Date().toISOString()): User {
  return {
    id: m.user_id,
    username: m.username ?? "",
    displayName: m.display_name || m.username || "KikBite user",
    email: m.email || undefined,
    emailVerified: m.email ? m.email_verified === true : undefined,
    pendingEmail: m.pending_email || undefined,
    // verified OTP number wins; otherwise the optional unverified contact number
    phone: m.phone_number || m.contact_phone || undefined,
    bio: m.about || undefined,
    avatarUrl: m.avatar_media_id ? avatarLink(m.user_id, m.avatar_media_id) : undefined,
    presence: "online",
    createdAt,
  };
}

/** auth responses: tokens block (backend auth.TokenPair). */
export interface ApiTokens {
  access_token: string;
  refresh_token: string;
  access_expires_at: string;
  refresh_expires_at: string;
}

/** Public profile as returned by GET /users/{id} and /users/search (backend users.PublicProfile). */
export interface ApiPublicProfile {
  user_id: string;
  username: string;
  display_name?: string;
  about?: string;
  avatar_media_id?: string;
}

export function fromPublic(p: ApiPublicProfile): User {
  return { id: p.user_id, username: p.username, displayName: p.display_name || p.username, bio: p.about || undefined, avatarUrl: p.avatar_media_id ? avatarLink(p.user_id, p.avatar_media_id) : undefined, presence: "offline", createdAt: new Date(0).toISOString() };
}

/** Friend row from /v1/friends (backend friends.Relation). */
export interface ApiRelation {
  user: { id: string; username: string; display_name: string; about?: string };
  status: Friend["status"];
  since?: string;
}

export function toFriend(r: ApiRelation): Friend {
  return {
    user: { id: r.user.id, username: r.user.username, displayName: r.user.display_name || r.user.username, bio: r.user.about || undefined, presence: "offline", createdAt: new Date(0).toISOString() },
    status: r.status,
    since: r.since,
  };
}

export interface ApiNotification {
  id: string;
  kind: AppNotification["kind"];
  title: string;
  body: string;
  actor_id?: string;
  conversation_id?: string;
  read: boolean;
  created_at: string;
}

export function toNotification(n: ApiNotification): AppNotification {
  return { id: n.id, kind: n.kind, title: n.title, body: n.body, actorId: n.actor_id, conversationId: n.conversation_id, read: n.read, createdAt: n.created_at };
}
