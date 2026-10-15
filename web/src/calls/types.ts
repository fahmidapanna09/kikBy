import type { ID } from "@/types/models";

export type CallKind = "voice" | "video";

/** A call as the UI sees it (backend chat.Call). */
export interface CallInfo {
  id: ID;
  conversationId: ID;
  initiatorId: ID;
  kind: CallKind;
  status: "ringing" | "active" | "ended" | "missed" | "declined" | "cancelled";
  endReason?: string;
}

/** Opaque WebRTC signaling payload relayed by the server between two participants. */
export type CallSignalData =
  | { type: "offer" | "answer"; sdp: string }
  | { type: "candidate"; candidate: RTCIceCandidateInit };

export interface PeerView {
  userId: ID;
  state: "connecting" | "connected" | "failed";
  stream: MediaStream | null;
}

export interface ActiveCall {
  callId: ID | null; // null until the server confirms (call.joined)
  conversationId: ID;
  kind: CallKind;
  confirmed: boolean;
  phase: "starting" | "ringing" | "connecting" | "connected";
  startedAt: number | null;
  peers: Record<ID, PeerView>;
  localStream: MediaStream | null;
  hasVideo: boolean;
  micOn: boolean;
  camOn: boolean;
}

export interface IncomingCall {
  call: CallInfo;
  fromUserId: ID;
}
