import { memo, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
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
  const [viewing, setViewing] = useState(false);
  if (a.kind === "image" && href && !failed) {
    return (
      <>
        <button type="button" className="att-img" onClick={() => setViewing(true)} aria-label={`Open photo ${a.name}`}>
          <img src={href} alt={a.name} loading="lazy" decoding="async" data-thumb={isThumb || undefined} onError={() => setFailed(true)} />
        </button>
        {viewing && <MediaViewer a={a} startUrl={href} thumb={isThumb} onClose={() => setViewing(false)} />}
      </>
    );
  }
  if (a.kind === "video" && href && !failed) {
    // Plays right inside the chat (tap for the player controls); the expand button opens it big, still inside the app.
    return (
      <div className="att-videowrap">
        <video className="att-video" src={href} controls preload="metadata" playsInline controlsList="nodownload" aria-label={a.name} onError={() => setFailed(true)} />
        <button type="button" className="att-expand" onClick={() => setViewing(true)} aria-label="Open video full screen"><Icon name="eye" size={16} /></button>
        {viewing && <MediaViewer a={a} startUrl={href} thumb={false} onClose={() => setViewing(false)} />}
      </div>
    );
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

/**
 * Full-screen photo / video viewer that lives INSIDE the app (like WhatsApp / Messenger): no new tab, no public link
 * opened in a browser. Back button / Esc / the X close it. Photos start from the small preview and swap to the full file.
 */
function MediaViewer({ a, startUrl, thumb, onClose }: { a: Attachment; startUrl: string; thumb: boolean; onClose: () => void }) {
  const [src, setSrc] = useState(startUrl);
  const [bad, setBad] = useState(false);
  useEffect(() => {
    if (!thumb) return;
    let live = true;
    services.uploads.resolveUrl?.(a.id).then((u) => live && setSrc(u), () => {});
    return () => {
      live = false;
    };
  }, [a.id, thumb]);
  useEffect(() => {
    // The phone's Back button should close the viewer, not leave the chat.
    let closedByBack = false;
    try {
      history.pushState({ kbViewer: 1 }, "");
    } catch {
      /* ignore */
    }
    const onPop = () => {
      closedByBack = true;
      onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("popstate", onPop);
    window.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("popstate", onPop);
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
      if (!closedByBack && (history.state as { kbViewer?: number } | null)?.kbViewer) {
        try {
          history.back();
        } catch {
          /* ignore */
        }
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return createPortal(
    <div className="media-viewer" role="dialog" aria-modal="true" aria-label={a.name} onClick={onClose}>
      <div className="mv-bar" onClick={(e: { stopPropagation(): void }) => e.stopPropagation()}>
        <button type="button" className="mv-btn" onClick={onClose} aria-label="Close"><Icon name="close" size={22} /></button>
        <span className="mv-title truncate">{a.name}</span>
        <a className="mv-btn" href={src} download={a.name} aria-label="Save" onClick={(e: { stopPropagation(): void }) => e.stopPropagation()}><Icon name="download" size={20} /></a>
      </div>
      <div className="mv-stage" onClick={(e: { stopPropagation(): void }) => e.stopPropagation()}>
        {bad ? (
          <p className="mv-error">This file could not be loaded.</p>
        ) : a.kind === "video" ? (
          <video src={src} controls autoPlay playsInline onError={() => setBad(true)} />
        ) : (
          <img src={src} alt={a.name} onError={() => setBad(true)} />
        )}
      </div>
    </div>,
    document.body,
  );
}
