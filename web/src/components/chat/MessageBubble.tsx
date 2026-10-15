import { memo, useEffect, useRef, useState } from "react";
import { services } from "@/services";
import { Icon } from "@/icons/Icon";
import { cn, formatBytes, formatTime, linkify, safeUrl } from "@/lib/utils";
import type { Attachment, Message, User } from "@/types/models";

interface Props {
  message: Message;
  mine: boolean;
  sender?: User;
  showSender: boolean; // group chats: first message of a run from someone else
  repliedTo?: Message;
  repliedToName?: string;
  firstInRun: boolean;
  lastInRun: boolean;
  highlighted: boolean;
  meId: string;
  onOpenMenu: (m: Message, x: number, y: number) => void;
  onJumpTo: (id: string) => void;
  onRetry: (m: Message) => void;
  onDiscard: (m: Message) => void;
  onReact: (m: Message, emoji: string) => void;
}

function Status({ m }: { m: Message }) {
  switch (m.status) {
    case "sending":
      return <Icon name="clock" size={12} label="Sending" />;
    case "sent":
      return <Icon name="check" size={12} label="Sent" />;
    case "delivered":
      return <Icon name="check-double" size={12} label="Delivered" />;
    case "read":
      return <Icon name="check-double" size={12} className="read" label="Read" />;
    default:
      return <Icon name="alert" size={12} className="fail" label="Failed to send" />;
  }
}

function Body({ text }: { text: string }) {
  // Plain text + auto-linked URLs. Everything is a React text node (escaped); hrefs are http(s)-only.
  return (
    <span className="msg-text">
      {linkify(text).map((p, i) =>
        p.href && safeUrl(p.href) ? (
          <a key={i} href={safeUrl(p.href)} target="_blank" rel="noopener noreferrer nofollow">
            {p.text}
          </a>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </span>
  );
}

export const MessageBubble = memo(function MessageBubble(p: Props) {
  const { message: m, mine } = p;
  const deleted = !!m.deletedAt;
  const open = (x: number, y: number) => p.onOpenMenu(m, x, y);
  // Long-press (touch) opens the same menu as right-click.
  const press = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cancelPress = () => {
    if (press.current) clearTimeout(press.current);
    press.current = null;
  };

  return (
    <div
      className={cn("msg-row", mine ? "out" : "in", p.firstInRun && "first", p.highlighted && "flash")}
      id={`msg-${m.id}`}
      data-mid={m.id}
      onContextMenu={(e: any) => {
        e.preventDefault();
        if (!deleted && m.status !== "sending") open(e.clientX, e.clientY);
      }}
      onTouchStart={(e: any) => {
        const t = e.touches?.[0];
        if (!t || deleted || m.status === "sending") return;
        cancelPress();
        press.current = setTimeout(() => open(t.clientX, t.clientY), 450);
      }}
      onTouchMove={cancelPress}
      onTouchEnd={cancelPress}
      onTouchCancel={cancelPress}
    >
      {p.showSender && p.sender && <span className="msg-sender">{p.sender.displayName}</span>}
      <div className="bubble-wrap">
        <div className={cn("bubble", mine ? "out" : "in", p.lastInRun && "tail", deleted && "deleted")}>
          {deleted ? (
            <span className="msg-deleted"><Icon name="block" size={14} /> This message was deleted</span>
          ) : (
            <>
              {m.forwardedFrom && <span className="msg-fwd"><Icon name="forward" size={12} /> Forwarded</span>}
              {p.repliedTo !== undefined || m.replyToId ? (
                <button type="button" className={cn("reply-ref", !mine && "in-bubble")} onClick={() => m.replyToId && p.onJumpTo(m.replyToId)} aria-label="Go to replied message">
                  <b>{p.repliedToName ?? "Message"}</b>
                  <span className="truncate">{p.repliedTo ? (p.repliedTo.deletedAt ? "Deleted message" : p.repliedTo.text || "Attachment") : "Original message not loaded"}</span>
                </button>
              ) : null}
              {m.attachments.map((a) => <AttachmentView key={a.id} a={a} />)}
              {m.text && <Body text={m.text} />}
            </>
          )}
          <span className="msg-meta">
            {m.editedAt && !deleted && <span>edited</span>}
            <time dateTime={m.createdAt}>{formatTime(m.createdAt)}</time>
            {mine && !deleted && <Status m={m} />}
          </span>
        </div>
        {!deleted && m.status !== "sending" && m.status !== "failed" && (
          <button
            type="button"
            className="msg-more"
            aria-label="Message actions"
            aria-haspopup="menu"
            onClick={(e: any) => {
              const r = e.currentTarget.getBoundingClientRect();
              open(mine ? r.left - 190 : r.right, r.bottom);
            }}
          >
            <Icon name="more" size={16} />
          </button>
        )}
      </div>
      {m.status === "failed" && (
        <div className="msg-failed" role="alert">
          <span>Not sent.</span>
          <button type="button" onClick={() => p.onRetry(m)}>Retry</button>
          <span aria-hidden="true">·</span>
          <button type="button" onClick={() => p.onDiscard(m)}>Delete</button>
        </div>
      )}
      {m.reactions.length > 0 && !deleted && (
        <div className="reactions" role="group" aria-label="Reactions">
          {m.reactions.map((r) => {
            const mineR = r.userIds.includes(p.meId);
            return (
              <button key={r.emoji} type="button" className={cn("react-chip", mineR && "mine")} aria-pressed={mineR} aria-label={`${r.emoji} ${r.userIds.length}${mineR ? ", you reacted" : ""}`} onClick={() => p.onReact(m, r.emoji)}>
                <span aria-hidden="true">{r.emoji}</span> {r.userIds.length}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
});

/**
 * One attachment. Mock/just-uploaded attachments already carry a url; ones loaded from the server don't (signed links
 * expire), so the link is fetched when the bubble is first rendered.
 */
function AttachmentView({ a }: { a: Attachment }) {
  const [url, setUrl] = useState(a.url);
  const [isThumb, setIsThumb] = useState(false);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    setUrl(a.url);
    setIsThumb(false);
    const up = services.uploads;
    if (a.url) return;
    let live = true;
    // Images ask for a small preview first; the full file is fetched when it is opened.
    const job = a.kind === "image" && up.resolveThumbnail ? up.resolveThumbnail(a.id).then((r) => live && (setUrl(r.url), setIsThumb(r.thumb))) : up.resolveUrl?.(a.id).then((u) => live && setUrl(u));
    job?.catch(() => live && setFailed(true));
    return () => {
      live = false;
    };
  }, [a.id, a.url, a.kind]);
  const href = safeUrl(url);
  const openFull = (e: any) => {
    if (!isThumb) return; // already the full file → normal link behaviour
    e.preventDefault();
    const w = window.open("", "_blank");
    if (w) w.opener = null;
    services.uploads.resolveUrl?.(a.id).then((u) => (w ? (w.location.href = u) : window.location.assign(u)), () => w?.close());
  };
  if (a.kind === "image" && href && !failed) {
    return (
      <a className="att-img" href={href} target="_blank" rel="noopener noreferrer" onClick={openFull}>
        <img src={href} alt={a.name} loading="lazy" decoding="async" data-thumb={isThumb || undefined} onError={() => setFailed(true)} />
      </a>
    );
  }
  if (a.kind === "video" && href && !failed) {
    return <video className="att-video" src={href} controls preload="metadata" playsInline aria-label={a.name} onError={() => setFailed(true)} />;
  }
  if (a.kind === "audio" && href && !failed) {
    return (
      <div className="att-audio">
        <span className="att-meta"><b className="truncate">{a.name}</b></span>
        <audio src={href} controls preload="none" aria-label={a.name} onError={() => setFailed(true)} />
      </div>
    );
  }
  return (
    <a className="att-file" href={href} download={a.name} target="_blank" rel="noopener noreferrer" aria-disabled={!href} data-state={failed ? "error" : href ? "ready" : "loading"}>
      <Icon name="doc" size={20} />
      <span className="att-meta"><b className="truncate">{a.name}</b><span>{failed ? "Unavailable" : formatBytes(a.sizeBytes)}</span></span>
    </a>
  );
}
