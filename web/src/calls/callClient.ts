import { ensureUsers } from "@/api/chatService";
import { toCall, type ApiCall } from "@/api/chatMappers";
import { toast } from "@/components/common/Toast";
import { sendRealtime } from "@/realtime";
import type { ServerEvent } from "@/realtime/events";
import { config } from "@/services/config";
import { http } from "@/services/http";
import { session } from "@/services/session";
import { store } from "@/state/store";
import type { ID } from "@/types/models";
import { callStore } from "./callStore";
import { startRingtone, stopRingtone } from "./ringtone";
import type { ActiveCall, CallInfo, CallKind, CallSignalData, PeerView } from "./types";

/**
 * Voice / video / group calls: WebRTC mesh (every participant connects directly to every other; the server only relays
 * offers, answers and ICE candidates and keeps call state). A mesh is right for a prototype and small groups (the server caps
 * a call at 4); bigger calls would move to an SFU (e.g. LiveKit) without changing this file's callers.
 *
 * Who offers: whoever JOINS sends an offer to everyone already in the call; the people already there only answer. If two
 * people join at the same moment and offer each other, the "perfect negotiation" rule resolves it (the peer with the
 * lexicographically smaller user id is polite and yields).
 *
 * One call per tab. Components talk to this module only through the exported functions and the `callStore`.
 */

interface Peer {
  userId: ID;
  pc: RTCPeerConnection;
  polite: boolean;
  makingOffer: boolean;
  ignoreOffer: boolean;
  restarted: boolean;
  queued: RTCIceCandidateInit[];
  chain: Promise<void>;
}

const peers = new Map<ID, Peer>();
let iceServers: RTCIceServer[] = [{ urls: ["stun:stun.l.google.com:19302"] }];
let confirmTimer: ReturnType<typeof setTimeout> | null = null;
let incomingTimer: ReturnType<typeof setTimeout> | null = null;
/** Signals that reach us after we asked to join but before the server's "joined" answer (they travel on separate channels). */
let pendingSignals: Array<{ callId: ID; fromUserId: ID; data: CallSignalData }> = [];
/** Conversations whose call was started and then cancelled before the server confirmed: leave as soon as it does. */
const abandoned = new Set<ID>();

const me = () => session.get()?.user.id ?? "";
export const callsAvailable = () => !config.mock || config.liveChat;
const nameOf = (id: ID) => store.get().users[id]?.displayName ?? "Someone";
const kindOfConv = (id: ID) => store.get().conversations.find((c) => c.id === id)?.kind;

function patchActive(p: Partial<ActiveCall>) {
  callStore.set((s) => (s.active ? { active: { ...s.active, ...p } } : {}));
}
function patchPeer(userId: ID, p: Partial<PeerView>) {
  callStore.set((s) => {
    if (!s.active) return {};
    const cur = s.active.peers[userId] ?? { userId, state: "connecting" as const, stream: null };
    return { active: { ...s.active, peers: { ...s.active.peers, [userId]: { ...cur, ...p } } } };
  });
}
function dropPeerView(userId: ID) {
  callStore.set((s) => {
    if (!s.active) return {};
    const { [userId]: _gone, ...rest } = s.active.peers;
    return { active: { ...s.active, peers: rest } };
  });
}
function setLive(conversationId: ID, v: { callId: ID; kind: CallKind } | null) {
  callStore.set((s) => {
    const live = { ...s.live };
    if (v) live[conversationId] = v;
    else delete live[conversationId];
    return { live };
  });
}

/* ───────── local media ───────── */

function mediaError(e: unknown): string {
  const name = (e as { name?: string } | null)?.name;
  if (name === "NotAllowedError" || name === "SecurityError") return "Allow microphone (and camera) access in your browser to make calls.";
  if (name === "NotFoundError" || name === "OverconstrainedError") return "No microphone found on this device.";
  if (name === "NotReadableError") return "Your microphone or camera is being used by another app.";
  return "Couldn't access your microphone.";
}

async function acquireMedia(kind: CallKind): Promise<{ stream: MediaStream; hasVideo: boolean }> {
  if (!navigator.mediaDevices?.getUserMedia) throw Object.assign(new Error("insecure"), { name: "SecurityError" });
  const audio = { echoCancellation: true, noiseSuppression: true, autoGainControl: true };
  if (kind === "video") {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio, video: { width: { ideal: 640 }, height: { ideal: 480 }, frameRate: { ideal: 24 } } });
      return { stream, hasVideo: true };
    } catch (e) {
      if ((e as { name?: string }).name === "NotAllowedError") throw e;
      toast.info("Camera unavailable — joining with audio only.");
    }
  }
  return { stream: await navigator.mediaDevices.getUserMedia({ audio }), hasVideo: false };
}

/* ───────── starting / answering ───────── */

function freshActive(conversationId: ID, kind: CallKind, callId: ID | null): ActiveCall {
  return { callId, conversationId, kind, confirmed: false, phase: callId ? "connecting" : "starting", startedAt: null, peers: {}, localStream: null, hasVideo: false, micOn: true, camOn: false };
}

function armConfirmTimer() {
  if (confirmTimer) clearTimeout(confirmTimer);
  confirmTimer = setTimeout(() => {
    if (callStore.get().active && !callStore.get().active?.confirmed) {
      const conv = callStore.get().active!.conversationId;
      teardown();
      abandoned.add(conv);
      toast.error("Couldn't connect the call. Check your connection and try again.");
    }
  }, 12000);
}

export async function startCall(conversationId: ID, kind: CallKind) {
  if (callStore.get().active) return;
  if (store.get().connection !== "open") return void toast.error("You're offline — can't start a call right now.");
  callStore.set({ active: freshActive(conversationId, kind, null) });
  try {
    const m = await acquireMedia(kind);
    if (!callStore.get().active) return m.stream.getTracks().forEach((t) => t.stop()); // cancelled while the permission prompt was open
    patchActive({ localStream: m.stream, hasVideo: m.hasVideo, camOn: m.hasVideo });
  } catch (e) {
    callStore.set({ active: null });
    return void toast.error(mediaError(e));
  }
  armConfirmTimer();
  sendRealtime({ type: "call:start", conversationId, kind });
}

async function answerCall(callId: ID, conversationId: ID, kind: CallKind, audioOnly = false) {
  if (callStore.get().active) return;
  if (store.get().connection !== "open") return void toast.error("You're offline — can't join the call right now.");
  callStore.set({ active: freshActive(conversationId, kind, callId) });
  try {
    // "Audio only" in a video call: we send no video but still receive everybody else's (see addLocalTracks).
    const m = await acquireMedia(audioOnly ? "voice" : kind);
    if (!callStore.get().active) return m.stream.getTracks().forEach((t) => t.stop());
    patchActive({ localStream: m.stream, hasVideo: m.hasVideo, camOn: m.hasVideo });
  } catch (e) {
    callStore.set({ active: null });
    return void toast.error(mediaError(e));
  }
  armConfirmTimer();
  sendRealtime({ type: "call:join", callId });
}

export function acceptIncoming(audioOnly = false) {
  const inc = callStore.get().incoming;
  if (!inc) return;
  stopRingtone();
  callStore.set({ incoming: null });
  void answerCall(inc.call.id, inc.call.conversationId, inc.call.kind, audioOnly);
}

export function rejectIncoming() {
  const inc = callStore.get().incoming;
  if (!inc) return;
  stopRingtone();
  callStore.set({ incoming: null });
  sendRealtime({ type: "call:reject", callId: inc.call.id });
}

export function joinLive(conversationId: ID, audioOnly = false) {
  const l = callStore.get().live[conversationId];
  if (l) void answerCall(l.callId, conversationId, l.kind, audioOnly);
}

/** Seed the "call in progress" banner when a chat is opened (events only reach us while we are online). */
export async function refreshLive(conversationId: ID) {
  if (!callsAvailable()) return;
  try {
    const r = await http<{ calls: ApiCall[] }>(`/conversations/${conversationId}/calls?limit=1`);
    const c = r.calls?.[0] ? toCall(r.calls[0]) : null;
    setLive(conversationId, c && (c.status === "ringing" || c.status === "active") ? { callId: c.id, kind: c.kind } : null);
  } catch {
    /* banner is a convenience */
  }
}

export function hangup() {
  const a = callStore.get().active;
  if (!a) return;
  if (a.callId) sendRealtime({ type: "call:leave", callId: a.callId });
  else if (confirmTimer) abandoned.add(a.conversationId); // call.start was sent but not confirmed yet
  teardown();
}

export function toggleMic() {
  const a = callStore.get().active;
  if (!a?.localStream) return;
  const on = !a.micOn;
  a.localStream.getAudioTracks().forEach((t) => (t.enabled = on));
  patchActive({ micOn: on });
}

export function toggleCam() {
  const a = callStore.get().active;
  if (!a?.localStream || !a.hasVideo) return;
  const on = !a.camOn;
  a.localStream.getVideoTracks().forEach((t) => (t.enabled = on));
  patchActive({ camOn: on });
}

function teardown() {
  if (confirmTimer) clearTimeout(confirmTimer);
  confirmTimer = null;
  peers.forEach((p) => {
    p.pc.onicecandidate = p.pc.ontrack = p.pc.onconnectionstatechange = p.pc.onnegotiationneeded = null;
    p.pc.close();
  });
  peers.clear();
  pendingSignals = [];
  callStore.get().active?.localStream?.getTracks().forEach((t) => t.stop());
  stopRingtone();
  callStore.set({ active: null });
}

/* ───────── peer connections ───────── */

const sendSignal = (toUserId: ID, data: CallSignalData) => {
  const callId = callStore.get().active?.callId;
  if (callId) sendRealtime({ type: "call:signal", callId, toUserId, data });
};

function addLocalTracks(peer: Peer) {
  const a = callStore.get().active;
  if (!a) return;
  const { pc } = peer;
  for (const t of a.localStream?.getTracks() ?? []) {
    if (!pc.getSenders().some((s) => s.track === t)) pc.addTrack(t, a.localStream as MediaStream);
  }
  // Without a camera we still want to SEE the others in a video call.
  if (a.kind === "video" && !a.hasVideo && !pc.getTransceivers().some((t) => t.receiver.track.kind === "video")) pc.addTransceiver("video", { direction: "recvonly" });
}

function markConnected() {
  const a = callStore.get().active;
  if (!a || a.phase === "connected") return;
  stopRingtone();
  patchActive({ phase: "connected", startedAt: a.startedAt ?? Date.now() });
}

function makePeer(userId: ID): Peer {
  const pc = new RTCPeerConnection({ iceServers });
  const peer: Peer = { userId, pc, polite: me() < userId, makingOffer: false, ignoreOffer: false, restarted: false, queued: [], chain: Promise.resolve() };
  const fallback = new MediaStream();
  pc.ontrack = (ev) => {
    let stream = ev.streams[0];
    if (!stream) {
      fallback.addTrack(ev.track);
      stream = fallback;
    }
    patchPeer(userId, { stream });
  };
  pc.onicecandidate = (ev) => {
    if (ev.candidate) sendSignal(userId, { type: "candidate", candidate: ev.candidate.toJSON() });
  };
  pc.onconnectionstatechange = () => {
    const st = pc.connectionState;
    if (st === "connected") {
      patchPeer(userId, { state: "connected" });
      markConnected();
    } else if (st === "failed") {
      patchPeer(userId, { state: "failed" });
      if (!peer.restarted) {
        peer.restarted = true;
        pc.restartIce();
      }
    } else if (st === "disconnected" || st === "connecting") {
      patchPeer(userId, { state: "connecting" });
    }
  };
  pc.onnegotiationneeded = async () => {
    try {
      peer.makingOffer = true;
      await pc.setLocalDescription();
      if (pc.localDescription) sendSignal(userId, { type: "offer", sdp: pc.localDescription.sdp });
    } catch (e) {
      console.warn("call: offer failed", e);
    } finally {
      peer.makingOffer = false;
    }
  };
  peers.set(userId, peer);
  patchPeer(userId, { state: "connecting" });
  return peer;
}

function closePeer(userId: ID) {
  const p = peers.get(userId);
  if (p) {
    p.pc.onicecandidate = p.pc.ontrack = p.pc.onconnectionstatechange = p.pc.onnegotiationneeded = null;
    p.pc.close();
    peers.delete(userId);
  }
  dropPeerView(userId);
}

async function flushQueued(peer: Peer) {
  const q = peer.queued.splice(0);
  for (const c of q) {
    try {
      await peer.pc.addIceCandidate(c);
    } catch (e) {
      if (!peer.ignoreOffer) console.warn("call: bad candidate", e);
    }
  }
}

async function onSignal(peer: Peer, data: CallSignalData) {
  const { pc } = peer;
  if (data.type === "offer") {
    const collision = peer.makingOffer || pc.signalingState !== "stable";
    peer.ignoreOffer = !peer.polite && collision;
    if (peer.ignoreOffer) return;
    await pc.setRemoteDescription({ type: "offer", sdp: data.sdp }); // implicitly rolls back our own pending offer when we are the polite side
    addLocalTracks(peer);
    await pc.setLocalDescription();
    if (pc.localDescription) sendSignal(peer.userId, { type: "answer", sdp: pc.localDescription.sdp });
    await flushQueued(peer);
  } else if (data.type === "answer") {
    if (pc.signalingState !== "have-local-offer") return;
    await pc.setRemoteDescription({ type: "answer", sdp: data.sdp });
    await flushQueued(peer);
  } else if (data.type === "candidate") {
    if (!pc.remoteDescription) peer.queued.push(data.candidate);
    else {
      try {
        await pc.addIceCandidate(data.candidate);
      } catch (e) {
        if (!peer.ignoreOffer) console.warn("call: bad candidate", e);
      }
    }
  }
}

/* ───────── server events ───────── */

const ERRORS: Record<string, string> = {
  call_full: "This call is full (up to 4 people).",
  call_ended: "That call has already ended.",
  call_blocked: "You can't call this person.",
  call_not_found: "That call no longer exists.",
  not_a_member: "You're not a member of this chat.",
  calls_disabled: "Calls aren't enabled on this server.",
};

export function handleCallEvent(e: ServerEvent) {
  switch (e.type) {
    case "call:incoming":
      return onIncoming(e.call, e.fromUserId);
    case "call:joined":
      return onJoined(e.call, e.participants, e.iceServers);
    case "call:participant_joined": {
      const a = callStore.get().active;
      if (!a?.confirmed || a.callId !== e.callId) return;
      patchPeer(e.userId, {});
      if (a.phase === "ringing") {
        stopRingtone();
        patchActive({ phase: "connecting" });
      }
      void ensureUsers([e.userId]);
      return;
    }
    case "call:participant_left": {
      const a = callStore.get().active;
      if (a?.callId === e.callId) closePeer(e.userId);
      return;
    }
    case "call:signal": {
      const a = callStore.get().active;
      if (a && !a.confirmed && a.callId === e.callId) {
        if (pendingSignals.length < 500) pendingSignals.push({ callId: e.callId, fromUserId: e.fromUserId, data: e.data });
        return;
      }
      if (a?.confirmed && a.callId === e.callId) routeSignal(e.fromUserId, e.data);
      return;
    }
    case "call:ended":
      return onEnded(e.call);
    case "call:dismissed":
      if (callStore.get().incoming?.call.id === e.callId) {
        stopRingtone();
        callStore.set({ incoming: null });
      }
      return;
    case "call:error": {
      const a = callStore.get().active;
      if (a && !a.confirmed) teardown();
      toast.error(ERRORS[e.code] ?? "The call couldn't be completed.");
      return;
    }
  }
}

function routeSignal(from: ID, data: CallSignalData) {
  let peer = peers.get(from);
  if (!peer) {
    peer = makePeer(from);
    void ensureUsers([from]);
  }
  const p = peer;
  p.chain = p.chain.then(() => onSignal(p, data)).catch((err) => console.warn("call: signal failed", err));
}

function onIncoming(call: CallInfo, from: ID) {
  setLive(call.conversationId, { callId: call.id, kind: call.kind });
  if (from === me()) return;
  void ensureUsers([from]);
  const busy = callStore.get().active || callStore.get().incoming;
  if (busy) {
    if (kindOfConv(call.conversationId) !== "group") sendRealtime({ type: "call:reject", callId: call.id });
    return;
  }
  callStore.set({ incoming: { call, fromUserId: from } });
  startRingtone("incoming");
  if (incomingTimer) clearTimeout(incomingTimer);
  incomingTimer = setTimeout(() => {
    if (callStore.get().incoming?.call.id === call.id) {
      stopRingtone();
      callStore.set({ incoming: null });
    }
  }, 60000);
}

function onJoined(call: CallInfo, participants: ID[], ice: RTCIceServer[]) {
  const a = callStore.get().active;
  if (!a || a.confirmed) {
    if (!a) {
      if (callStore.get().incoming?.call.id === call.id) {
        // answered on another tab/device of mine
        stopRingtone();
        callStore.set({ incoming: null });
      } else if (abandoned.delete(call.conversationId) && call.initiatorId === me()) {
        sendRealtime({ type: "call:leave", callId: call.id }); // I cancelled before the server confirmed
      }
    }
    return;
  }
  if (a.callId ? a.callId !== call.id : a.conversationId !== call.conversationId) return;
  if (confirmTimer) clearTimeout(confirmTimer);
  if (ice.length) iceServers = ice;
  patchActive({ callId: call.id, kind: call.kind, confirmed: true, phase: participants.length ? "connecting" : "ringing" });
  setLive(call.conversationId, { callId: call.id, kind: call.kind });
  if (participants.length) {
    stopRingtone();
    void ensureUsers(participants);
    for (const p of participants) {
      const peer = makePeer(p);
      addLocalTracks(peer); // adding tracks triggers negotiationneeded → we send the offer
      if (!callStore.get().active?.localStream?.getTracks().length) peer.pc.addTransceiver("audio", { direction: "recvonly" });
    }
  } else startRingtone("outgoing");
  const queued = pendingSignals.filter((x) => x.callId === call.id);
  pendingSignals = [];
  for (const x of queued) routeSignal(x.fromUserId, x.data);
}

function onEnded(call: CallInfo) {
  setLive(call.conversationId, null);
  const a = callStore.get().active;
  if (a?.callId === call.id) {
    teardown();
    if (call.status === "declined") toast.info("Call declined.");
    else if (call.status === "missed") toast.info("No answer.");
    return;
  }
  const inc = callStore.get().incoming;
  if (inc?.call.id === call.id) {
    stopRingtone();
    callStore.set({ incoming: null });
    if (call.status === "missed" || call.status === "cancelled") toast.info(`Missed ${call.kind} call from ${nameOf(inc.fromUserId)}.`);
  }
}
