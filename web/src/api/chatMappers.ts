import type { CallInfo } from "@/calls/types";
import type { Attachment, AttachmentKind, Conversation, Message, Reaction, User } from "@/types/models";
import { attachmentLabel } from "@/lib/utils";
import type { ServerEvent } from "@/realtime/events";
import type { ApiNotification } from "./mappers";
import { toNotification } from "./mappers";

/** Backend chat.Message (snake_case JSON). */
export interface ApiMessage {
  id: string;
  conversation_id: string;
  seq: number;
  sender_id: string;
  client_tmp_id?: string;
  type: string;
  body?: string;
  reply_to_id?: string;
  created_at: string;
  edited_at?: string;
  deleted_at?: string;
  status?: "sent" | "delivered" | "read";
  attachment_ids?: string[];
  attachments?: ApiAttachment[];
  forwarded_from_message_id?: string;
  reactions?: ApiReaction[];
}
/** Backend chat.AttachmentInfo — display metadata only; the download URL is fetched per id. */
export interface ApiAttachment {
  id: string;
  kind: string;
  mime_type: string;
  size_bytes: number;
  name?: string;
}
export interface ApiReaction {
  emoji: string;
  user_ids: string[];
}

/** Backend chat.GroupSummary. */
export interface ApiGroup {
  conversation_id: string;
  name: string;
  members?: Array<{ user_id: string; role: "owner" | "admin" | "member" }>;
}

/** Backend chat.ConversationSummary. */
export interface ApiConversation {
  id: string;
  type: "direct" | "group" | string;
  peer_user_id?: string;
  name?: string;
  last_message?: ApiMessage;
  unread_count: number;
  pinned_at?: string;
  archived_at?: string;
  muted_until?: string;
}

export const toReactions = (r?: ApiReaction[]): Reaction[] => (r ?? []).map((x) => ({ emoji: x.emoji, userIds: x.user_ids ?? [] }));

export const toAttachmentKind = (k: string): AttachmentKind => (k === "image" || k === "video" || k === "audio" ? k : k === "voice" ? "audio" : "file");

export function toAttachment(a: ApiAttachment): Attachment {
  return { id: a.id, kind: toAttachmentKind(a.kind), name: a.name || "File", url: "", mimeType: a.mime_type, sizeBytes: a.size_bytes ?? 0 };
}

/**
 * Attachments arrive as metadata only (no URL — signed links expire); the bubble resolves a link per id when shown.
 * A media-only message has no text; the chat list falls back to "Photo" / "File" etc.
 */
export function toMessage(m: ApiMessage): Message {
  const deleted = !!m.deleted_at;
  const attachments = deleted ? [] : (m.attachments ?? []).map(toAttachment);
  const mediaOnly = attachments.length > 0 || (m.attachment_ids?.length ?? 0) > 0;
  const text = deleted ? "" : m.body ?? (!mediaOnly && m.type !== "text" ? `Sent a ${m.type}` : "");
  return {
    id: m.id,
    clientId: m.client_tmp_id || undefined,
    conversationId: m.conversation_id,
    senderId: m.sender_id,
    text,
    attachments,
    replyToId: m.reply_to_id || undefined,
    forwardedFrom: m.forwarded_from_message_id || undefined,
    reactions: deleted ? [] : toReactions(m.reactions),
    status: m.status === "delivered" || m.status === "read" ? m.status : "sent",
    createdAt: m.created_at,
    editedAt: m.edited_at || undefined,
    deletedAt: m.deleted_at || undefined,
  };
}

/** Conversation summaries carry the last message's type but not its attachments. */
const previewFor = (type?: string): string => (type && type !== "text" ? attachmentLabel(type === "document" ? "file" : type === "voice" ? "audio" : type) : "");

export function toConversation(c: ApiConversation, meId: string, peer?: User): Conversation {
  const direct = c.type === "direct";
  const last = c.last_message ? toMessage(c.last_message) : undefined;
  const updatedAt = last?.createdAt ?? new Date(0).toISOString();
  return {
    id: c.id,
    kind: direct ? "direct" : "group",
    title: direct ? peer?.displayName ?? peer?.username ?? "Chat" : c.name ?? "Group",
    memberIds: direct && c.peer_user_id ? [meId, c.peer_user_id] : [meId],
    peerId: direct ? c.peer_user_id : undefined,
    lastMessage: last ? { id: last.id, text: last.deletedAt ? "This message was deleted" : last.text || previewFor(c.last_message?.type), senderId: last.senderId, createdAt: last.createdAt, status: last.status } : undefined,
    unreadCount: c.unread_count ?? 0,
    pinned: !!c.pinned_at,
    archived: !!c.archived_at,
    muted: !!c.muted_until && Date.parse(c.muted_until) > Date.now(),
    updatedAt,
  };
}

/** Backend chat.Call. */
export interface ApiCall {
  id: string;
  conversation_id: string;
  initiator_id: string;
  kind: string;
  status: string;
  end_reason?: string;
}
export const toCall = (c: ApiCall): CallInfo => ({
  id: c.id,
  conversationId: c.conversation_id,
  initiatorId: c.initiator_id,
  kind: c.kind === "video" ? "video" : "voice",
  status: (["ringing", "active", "ended", "missed", "declined", "cancelled"].includes(c.status) ? c.status : "ended") as CallInfo["status"],
  endReason: c.end_reason || undefined,
});

/* ───────── WebSocket wire format (backend chat.Envelope) ───────── */

export interface WireEnvelope {
  type: string;
  ts?: string;
  conversation_id?: string;
  payload?: unknown;
}

type P = Record<string, unknown>;

/** Server envelope → the UI's ServerEvent. Unknown or irrelevant frames (ack, pong, error, groups) → null. */
export function decodeEnvelope(env: WireEnvelope): ServerEvent | null {
  const conv = env.conversation_id ?? "";
  const p = (env.payload ?? {}) as P;
  switch (env.type) {
    case "message.new":
      return { type: "message:new", message: toMessage(p as unknown as ApiMessage) };
    case "message.edited":
      return { type: "message:update", message: toMessage((p as { message: ApiMessage }).message) };
    case "message.deleted":
      return { type: "message:delete", conversationId: conv, messageId: String(p.message_id), deletedAt: env.ts ?? new Date().toISOString() };
    case "message.delivered":
      return { type: "message:delivered", conversationId: conv, messageId: String(p.message_id) };
    case "message.read":
      return { type: "message:read", conversationId: conv, userId: String(p.user_id), upToMessageId: String(p.message_id) };
    case "message.reaction":
      return { type: "message:reactions", conversationId: conv, messageId: String(p.message_id), reactions: toReactions(p.reactions as ApiReaction[]) };
    case "typing.start":
      return { type: "user:typing", conversationId: conv, userId: String(p.user_id) };
    case "typing.stop":
      return { type: "user:stop_typing", conversationId: conv, userId: String(p.user_id) };
    case "presence.update":
      return p.online
        ? { type: "user:online", userId: String(p.user_id) }
        : { type: "user:offline", userId: String(p.user_id), lastSeenAt: String(p.last_seen ?? env.ts ?? new Date().toISOString()) };
    case "group.updated":
    case "group.member_added":
      return { type: "conversations:refresh" };
    case "group.member_removed":
      return { type: "group:member_removed", conversationId: conv, userId: String(p.user_id) };
    case "call.incoming":
      return { type: "call:incoming", call: toCall(p.call as ApiCall), fromUserId: String(p.from_user_id) };
    case "call.joined":
      return { type: "call:joined", call: toCall(p.call as ApiCall), participants: ((p.participants as string[]) ?? []).map(String), iceServers: (p.ice_servers as RTCIceServer[]) ?? [] };
    case "call.participant_joined":
      return { type: "call:participant_joined", callId: String(p.call_id), userId: String(p.user_id) };
    case "call.participant_left":
      return { type: "call:participant_left", callId: String(p.call_id), userId: String(p.user_id) };
    case "call.signal":
      return { type: "call:signal", callId: String(p.call_id), fromUserId: String(p.from_user_id), data: p.data as import("@/calls/types").CallSignalData };
    case "call.ended":
      return { type: "call:ended", call: toCall(p.call as ApiCall) };
    case "call.dismissed":
      return { type: "call:dismissed", callId: String(p.call_id) };
    case "error":
      return typeof p.code === "string" && (p.code.startsWith("call_") || p.code === "calls_disabled" || p.code === "not_a_member")
        ? { type: "call:error", code: p.code, message: String(p.message ?? "") }
        : null;
    case "notification.new":
      return { type: "notification:new", notification: toNotification((p as { notification: ApiNotification }).notification) };
    default:
      return null;
  }
}

export function encodeClientEvent(e: import("@/realtime/events").ClientEvent): string {
  const base = { event_id: `w${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`, ts: new Date().toISOString(), version: 1 };
  switch (e.type) {
    case "typing:start":
      return JSON.stringify({ ...base, type: "typing.start", conversation_id: e.conversationId, payload: {} });
    case "typing:stop":
      return JSON.stringify({ ...base, type: "typing.stop", conversation_id: e.conversationId, payload: {} });
    case "message:read":
      return JSON.stringify({ ...base, type: "message.read", conversation_id: e.conversationId, payload: { message_id: e.upToMessageId } });
    case "message:delivered":
      return JSON.stringify({ ...base, type: "message.delivered", conversation_id: e.conversationId, payload: { message_id: e.messageId } });
    case "call:start":
      return JSON.stringify({ ...base, type: "call.start", conversation_id: e.conversationId, payload: { kind: e.kind } });
    case "call:join":
      return JSON.stringify({ ...base, type: "call.join", payload: { call_id: e.callId } });
    case "call:leave":
      return JSON.stringify({ ...base, type: "call.leave", payload: { call_id: e.callId } });
    case "call:reject":
      return JSON.stringify({ ...base, type: "call.reject", payload: { call_id: e.callId } });
    case "call:signal":
      return JSON.stringify({ ...base, type: "call.signal", payload: { call_id: e.callId, to_user_id: e.toUserId, data: e.data } });
  }
}
