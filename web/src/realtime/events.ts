import type { CallInfo, CallKind, CallSignalData } from "@/calls/types";
import type { AppNotification, ID, Message, MessageStatus, Reaction } from "@/types/models";

/** Server → client events. This is the whole real-time contract the UI layer knows about. */
export type ServerEvent =
  | { type: "message:new"; message: Message }
  | { type: "message:update"; message: Message }
  | { type: "message:delete"; conversationId: ID; messageId: ID; deletedAt: string }
  | { type: "message:read"; conversationId: ID; userId: ID; upToMessageId?: ID }
  | { type: "message:delivered"; conversationId: ID; messageId: ID }
  | { type: "message:reactions"; conversationId: ID; messageId: ID; reactions: Reaction[] }
  | { type: "user:online"; userId: ID }
  | { type: "user:offline"; userId: ID; lastSeenAt: string }
  | { type: "user:typing"; conversationId: ID; userId: ID }
  | { type: "user:stop_typing"; conversationId: ID; userId: ID }
  | { type: "notification:new"; notification: AppNotification }
  /** A group was created/renamed or its member list changed: re-read the conversation list. */
  | { type: "conversations:refresh" }
  | { type: "group:member_removed"; conversationId: ID; userId: ID }
  /** Calls (WebRTC signaling). */
  | { type: "call:incoming"; call: CallInfo; fromUserId: ID }
  | { type: "call:joined"; call: CallInfo; participants: ID[]; iceServers: RTCIceServer[] }
  | { type: "call:participant_joined"; callId: ID; userId: ID }
  | { type: "call:participant_left"; callId: ID; userId: ID }
  | { type: "call:signal"; callId: ID; fromUserId: ID; data: CallSignalData }
  | { type: "call:ended"; call: CallInfo }
  | { type: "call:dismissed"; callId: ID }
  | { type: "call:error"; code: string; message: string };

export type ServerEventType = ServerEvent["type"];
export type ConnectionState = "idle" | "connecting" | "open" | "reconnecting" | "closed";

/** Client → server events. */
export type ClientEvent =
  | { type: "typing:start"; conversationId: ID }
  | { type: "typing:stop"; conversationId: ID }
  | { type: "message:read"; conversationId: ID; upToMessageId: ID }
  | { type: "message:delivered"; conversationId: ID; messageId: ID }
  | { type: "call:start"; conversationId: ID; kind: CallKind }
  | { type: "call:join"; callId: ID }
  | { type: "call:leave"; callId: ID }
  | { type: "call:reject"; callId: ID }
  | { type: "call:signal"; callId: ID; toUserId: ID; data: CallSignalData };

export type { MessageStatus };

/** Transport abstraction: WebSocket in production, MockSocket in dev. Nothing in UI imports a concrete socket. */
export interface SocketClient {
  connect(): void;
  disconnect(): void;
  send(event: ClientEvent): void;
  onEvent(handler: (e: ServerEvent) => void): () => void;
  onState(handler: (s: ConnectionState) => void): () => void;
  readonly state: ConnectionState;
}
