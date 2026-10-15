import { Avatar } from "@/components/common/Avatar";
import { IconButton } from "@/components/common/Button";
import { Icon } from "@/icons/Icon";
import { callsAvailable, startCall } from "@/calls/callClient";
import { useCalls } from "@/calls/callStore";
import { Link } from "@/lib/router";
import { formatLastSeen } from "@/lib/utils";
import { useApp } from "@/state/store";
import type { Conversation } from "@/types/models";

export function ChatHeader({ conv, onToggleDetails, detailsOpen }: { conv: Conversation; onToggleDetails: () => void; detailsOpen: boolean }) {
  const peer = useApp((s) => (conv.peerId ? s.users[conv.peerId] : undefined));
  const typing = useApp((s) => s.typing[conv.id]);
  const users = useApp((s) => s.users);
  const names = (typing ?? []).map((id) => users[id]?.displayName.split(" ")[0] ?? "Someone");
  const inCall = useCalls((s) => !!s.active || !!s.incoming);
  const typingNow = names.length > 0;
  const status = typingNow
    ? conv.kind === "group" ? `${names.join(", ")} typing…` : "typing…"
    : conv.kind === "group" ? `${conv.memberIds.length} members` : peer ? formatLastSeen(peer) : "";
  return (
    <header className="chat-header">
      <Link to="/chats" className="icon-btn back-btn" aria-label="Back to chats"><Icon name="back" /></Link>
      <button type="button" className="chat-title-btn" onClick={onToggleDetails} aria-label={`${conv.title} — open details`}>
        <Avatar name={conv.title} id={conv.peerId ?? conv.id} src={conv.avatarUrl} size={40} group={conv.kind === "group"} presence={conv.kind === "direct" ? peer?.presence : undefined} tone={conv.avatarTone} />
        <span className="hdr-text">
          <span className="name truncate">{conv.title}</span>
          <span className={`meta truncate${typingNow || peer?.presence === "online" ? " live" : ""}`} aria-live="polite">{status}</span>
        </span>
      </button>
      {callsAvailable() && (
        <>
          <IconButton label={conv.kind === "group" ? "Start group voice call" : "Voice call"} disabled={inCall} onClick={() => void startCall(conv.id, "voice")}><Icon name="phone" /></IconButton>
          <IconButton label={conv.kind === "group" ? "Start group video call" : "Video call"} disabled={inCall} onClick={() => void startCall(conv.id, "video")}><Icon name="video" /></IconButton>
        </>
      )}
      <IconButton label={detailsOpen ? "Hide details" : "Show details"} active={detailsOpen} aria-expanded={detailsOpen} onClick={onToggleDetails}><Icon name="info" /></IconButton>
    </header>
  );
}
