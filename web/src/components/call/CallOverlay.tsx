import { useEffect, useRef, useState } from "react";
import type { RefObject } from "react";
import { Avatar } from "@/components/common/Avatar";
import { acceptIncoming, hangup, rejectIncoming, toggleCam, toggleMic } from "@/calls/callClient";
import { useCalls } from "@/calls/callStore";
import type { ActiveCall, PeerView } from "@/calls/types";
import { Icon } from "@/icons/Icon";
import { session } from "@/services/session";
import { useApp } from "@/state/store";

/** Mounted once in the app shell: the incoming-call card and the full-screen call. */
export function CallOverlay() {
  const incoming = useCalls((s) => s.incoming);
  const active = useCalls((s) => s.active);
  return (
    <>
      {incoming && !active && <IncomingCard />}
      {active && <CallScreen call={active} />}
    </>
  );
}

function useCallTitle(conversationId: string, fallbackUserId?: string): { title: string; id: string; group: boolean } {
  const conv = useApp((s) => s.conversations.find((c) => c.id === conversationId));
  const users = useApp((s) => s.users);
  if (conv?.kind === "group") return { title: conv.title, id: conv.id, group: true };
  const peerId = conv?.peerId ?? fallbackUserId ?? "";
  return { title: conv?.title ?? users[peerId]?.displayName ?? "Call", id: peerId || conversationId, group: false };
}

function IncomingCard() {
  const inc = useCalls((s) => s.incoming);
  const users = useApp((s) => s.users);
  const t = useCallTitle(inc?.call.conversationId ?? "", inc?.fromUserId);
  if (!inc) return null;
  const caller = users[inc.fromUserId]?.displayName ?? "Someone";
  return (
    <div className="call-incoming" role="alertdialog" aria-label={`Incoming ${inc.call.kind} call`} aria-live="assertive">
      <Avatar name={t.title} id={t.id} size={48} group={t.group} />
      <div className="call-incoming-text">
        <strong className="truncate">{t.group ? t.title : caller}</strong>
        <span className="truncate">{t.group ? `${caller} started a group ${inc.call.kind} call` : `Incoming ${inc.call.kind} call…`}</span>
      </div>
      <button type="button" className="call-btn decline" aria-label="Decline call" onClick={rejectIncoming}><Icon name="phone-off" /></button>
      {inc.call.kind === "video" && <button type="button" className="call-btn audio" aria-label="Answer with audio only" title="Answer with audio only" onClick={() => acceptIncoming(true)}><Icon name="phone" /></button>}
      <button type="button" className="call-btn accept" aria-label="Accept call" onClick={() => acceptIncoming()} autoFocus><Icon name={inc.call.kind === "video" ? "video" : "phone"} /></button>
    </div>
  );
}

function useDuration(startedAt: number | null): string {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!startedAt) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [startedAt]);
  if (!startedAt) return "";
  const s = Math.max(0, Math.floor((now - startedAt) / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

function StatusLine({ call }: { call: ActiveCall }) {
  const dur = useDuration(call.startedAt);
  const text =
    call.phase === "starting" ? "Starting…" : call.phase === "ringing" ? "Ringing…" : call.phase === "connecting" ? "Connecting…" : dur;
  return <span className="call-status" role="timer" aria-live="off">{text}</span>;
}

function CallScreen({ call }: { call: ActiveCall }) {
  const t = useCallTitle(call.conversationId);
  const meId = session.get()?.user.id ?? "";
  const users = useApp((s) => s.users);
  const peerIds = Object.keys(call.peers);
  const video = call.kind === "video";
  const pip = video && peerIds.length === 1;
  return (
    <div className="call-screen" role="dialog" aria-modal="true" aria-label={`${call.kind === "video" ? "Video" : "Voice"} call with ${t.title}`} data-kind={call.kind}>
      <header className="call-head">
        <div className="call-head-text">
          <strong className="truncate">{t.title}</strong>
          <StatusLine call={call} />
        </div>
        {call.phase === "connected" && peerIds.length > 0 && <span className="call-count" aria-label={`${peerIds.length + 1} people in call`}><Icon name="users" size={16} /> {peerIds.length + 1}</span>}
      </header>

      <div className="call-grid" data-count={peerIds.length + (pip ? 0 : 1)}>
        {peerIds.length === 0 && (
          <div className="call-wait">
            <Avatar name={t.title} id={t.id} size={96} group={t.group} />
            <p>{call.phase === "starting" ? "Starting call…" : call.phase === "ringing" ? `Calling ${t.title}…` : "Joining…"}</p>
          </div>
        )}
        {peerIds.map((id) => (
          <Tile key={id} peer={call.peers[id]} name={users[id]?.displayName ?? "Participant"} video={video} />
        ))}
        {!pip && <LocalTile call={call} name="You" />}
      </div>
      {pip && <LocalTile call={call} name="You" pip />}

      <footer className="call-controls">
        <button type="button" className={`call-btn${call.micOn ? "" : " off"}`} aria-label={call.micOn ? "Mute microphone" : "Unmute microphone"} aria-pressed={!call.micOn} onClick={toggleMic}>
          <Icon name={call.micOn ? "mic" : "mic-off"} />
        </button>
        {video && (
          <button type="button" className={`call-btn${call.camOn ? "" : " off"}`} aria-label={call.camOn ? "Turn camera off" : "Turn camera on"} aria-pressed={!call.camOn} disabled={!call.hasVideo} onClick={toggleCam}>
            <Icon name={call.camOn ? "video" : "video-off"} />
          </button>
        )}
        <button type="button" className="call-btn decline" aria-label="End call" onClick={hangup} data-meid={meId}><Icon name="phone-off" /></button>
      </footer>
    </div>
  );
}

function useStream(ref: RefObject<HTMLMediaElement | null>, stream: MediaStream | null) {
  useEffect(() => {
    const el = ref.current;
    if (el && el.srcObject !== stream) el.srcObject = stream;
  }, [ref, stream]);
}

function Tile({ peer, name, video }: { peer: PeerView; name: string; video: boolean }) {
  const ref = useRef<HTMLVideoElement>(null);
  useStream(ref, peer.stream);
  const hasVideo = video && !!peer.stream && peer.stream.getVideoTracks().length > 0;
  return (
    <figure className={`call-tile${hasVideo ? "" : " audio-only"}`} data-state={peer.state} data-user={peer.userId}>
      <video ref={ref} autoPlay playsInline className="call-video" aria-hidden={!hasVideo} data-remote />
      {!hasVideo && <Avatar name={name} id={peer.userId} size={72} />}
      <figcaption>
        <span className="truncate">{name}</span>
        {peer.state !== "connected" && <span className="call-tile-state">{peer.state === "failed" ? "Connection lost" : "Connecting…"}</span>}
      </figcaption>
    </figure>
  );
}

function LocalTile({ call, name, pip }: { call: ActiveCall; name: string; pip?: boolean }) {
  const ref = useRef<HTMLVideoElement>(null);
  useStream(ref, call.localStream);
  const showVideo = call.kind === "video" && call.hasVideo && call.camOn;
  return (
    <figure className={`call-tile local${pip ? " pip" : ""}${showVideo ? "" : " audio-only"}`}>
      <video ref={ref} autoPlay playsInline muted className="call-video mirror" aria-hidden="true" data-local />
      {!showVideo && <Avatar name={name} id="me" size={pip ? 40 : 72} />}
      <figcaption><span className="truncate">{name}</span>{!call.micOn && <Icon name="mic-off" size={14} label="Muted" />}</figcaption>
    </figure>
  );
}
