import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { EmptyState, ErrorState } from "@/components/common/EmptyState";
import { Skeleton } from "@/components/common/Skeleton";
import { Icon } from "@/icons/Icon";
import { formatDayLabel } from "@/lib/utils";
import { loadOlder, openThread } from "@/state/messages";
import { useApp } from "@/state/store";
import type { Conversation, Message } from "@/types/models";
import { MessageBubble } from "./MessageBubble";

interface Props {
  conv: Conversation;
  meId: string;
  onOpenMenu: (m: Message, x: number, y: number) => void;
  onRetry: (m: Message) => void;
  onDiscard: (m: Message) => void;
  onReact: (m: Message, emoji: string) => void;
}

const NEAR_BOTTOM = 120;

type Row =
  | { kind: "day"; key: string; label: string }
  | { kind: "unread"; key: string }
  | { kind: "msg"; key: string; m: Message; first: boolean; last: boolean };

export function MessageList({ conv, meId, onOpenMenu, onRetry, onDiscard, onReact }: Props) {
  const thread = useApp((s) => s.threads[conv.id]);
  const users = useApp((s) => s.users);
  const typing = useApp((s) => s.typing[conv.id]);
  const scroller = useRef<HTMLDivElement | null>(null);
  const stick = useRef(true); // true while the user is at/near the bottom
  const prevHeight = useRef(0);
  const prevLen = useRef(0);
  const prevFirstId = useRef<string | undefined>(undefined);
  const prevLastId = useRef<string | undefined>(undefined);
  const [newBelow, setNewBelow] = useState(0);
  const [flashId, setFlashId] = useState<string | null>(null);
  const initialised = useRef(false);

  const items = thread?.items ?? [];
  const firstUnreadId = thread?.firstUnreadId;
  const byId = useMemo(() => new Map(items.map((m) => [m.id, m])), [items]);

  const rows = useMemo<Row[]>(() => {
    const out: Row[] = [];
    let lastDay = "";
    let unreadShown = false;
    items.forEach((m, i) => {
      const day = new Date(m.createdAt).toDateString();
      if (day !== lastDay) {
        out.push({ kind: "day", key: `d-${day}`, label: formatDayLabel(m.createdAt) });
        lastDay = day;
      }
      if (!unreadShown && firstUnreadId && m.id === firstUnreadId) {
        out.push({ kind: "unread", key: "unread" });
        unreadShown = true;
      }
      const prev = items[i - 1];
      const next = items[i + 1];
      const sameRun = (a?: Message) => !!a && a.senderId === m.senderId && new Date(a.createdAt).toDateString() === day && !a.deletedAt === !m.deletedAt;
      out.push({ kind: "msg", key: m.id, m, first: !(prev && sameRun(prev)), last: !(next && next.senderId === m.senderId && new Date(next.createdAt).toDateString() === day) });
    });
    return out;
  }, [items, firstUnreadId]);

  const scrollToBottom = useCallback((smooth = false) => {
    const el = scroller.current;
    if (el) el.scrollTo({ top: el.scrollHeight, behavior: smooth ? "smooth" : "auto" });
    setNewBelow(0);
  }, []);

  // Reset when switching conversations.
  useEffect(() => {
    initialised.current = false;
    stick.current = true;
    prevLen.current = 0;
    prevFirstId.current = undefined;
    prevLastId.current = undefined;
    setNewBelow(0);
  }, [conv.id]);

  // Layout effect: runs before paint so there is no flash of a wrong scroll position.
  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el || thread?.status !== "ready") return;
    const firstId = items[0]?.id;
    const lastId = items[items.length - 1]?.id;
    if (!initialised.current) {
      initialised.current = true;
      const div = firstUnreadId ? el.querySelector<HTMLElement>("[data-unread-divider]") : null;
      if (div) el.scrollTop = Math.max(0, div.getBoundingClientRect().top - el.getBoundingClientRect().top + el.scrollTop - 60);
      else el.scrollTop = el.scrollHeight;
      stick.current = !div;
    } else if (firstId !== prevFirstId.current && lastId === prevLastId.current) {
      // Older messages were prepended: keep the viewport anchored on what the reader was looking at.
      el.scrollTop += el.scrollHeight - prevHeight.current;
    } else if (lastId !== prevLastId.current) {
      const last = items[items.length - 1];
      const added = Math.max(1, items.length - prevLen.current);
      if (stick.current || last.senderId === meId) scrollToBottom(true);
      else setNewBelow((n) => n + added);
    }
    prevLen.current = items.length;
    prevFirstId.current = firstId;
    prevLastId.current = lastId;
    prevHeight.current = el.scrollHeight;
  });

  const onScroll = () => {
    const el = scroller.current;
    if (!el) return;
    stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM;
    if (stick.current && newBelow) setNewBelow(0);
    if (el.scrollTop < 80 && thread?.hasMore && !thread.loadingMore) {
      prevHeight.current = el.scrollHeight;
      void loadOlder(conv.id);
    }
  };

  const jumpTo = useCallback(
    async (id: string) => {
      // The replied-to message may be on an older page: keep paging until it's loaded (bounded).
      for (let i = 0; i < 20 && !document.getElementById(`msg-${id}`); i++) {
        const n = await loadOlder(conv.id);
        if (!n) break;
        await new Promise((r) => requestAnimationFrame(() => r(null)));
      }
      const target = document.getElementById(`msg-${id}`);
      if (!target) return;
      target.scrollIntoView({ block: "center", behavior: "smooth" });
      setFlashId(id);
      setTimeout(() => setFlashId(null), 1600);
    },
    [conv.id],
  );

  if (!thread || thread.status === "idle" || thread.status === "loading") {
    return (
      <div className="msg-scroll" role="status" aria-label="Loading messages">
        <div className="msg-pad">
          {[0, 1, 2, 3, 4].map((i) => (
            <div key={i} style={{ alignSelf: i % 2 ? "flex-end" : "flex-start", width: `${40 + ((i * 17) % 30)}%` }}>
              <Skeleton h={38 + (i % 3) * 10} className="" />
            </div>
          ))}
        </div>
      </div>
    );
  }
  if (thread.status === "error") return <ErrorState message={thread.error} onRetry={() => void openThread(conv.id)} />;
  if (items.length === 0) return <EmptyState icon="chat" title="Say hello 👋" message={`This is the start of your conversation with ${conv.title}.`} />;

  const typers = (typing ?? []).filter((u) => u !== meId);
  const nameOf = (id: string) => users[id]?.displayName ?? "Someone";

  return (
    <div className="msg-wrap">
      <div className="msg-scroll" ref={scroller} onScroll={onScroll} role="log" aria-live="polite" aria-relevant="additions" aria-label={`Conversation with ${conv.title}`}>
        <div className="msg-pad">
          {thread.loadingMore && <div className="older-loading" role="status"><span className="spinner" aria-hidden="true" /> Loading earlier messages…</div>}
          {!thread.hasMore && <div className="older-end">Beginning of conversation</div>}
          {rows.map((r) => {
            if (r.kind === "day") return <div key={r.key} className="day-sep"><span>{r.label}</span></div>;
            if (r.kind === "unread") return <div key={r.key} className="unread-sep" data-unread-divider role="separator" aria-label="New messages"><span>New messages</span></div>;
            const { m } = r;
            const replied = m.replyToId ? byId.get(m.replyToId) : undefined;
            return (
              <MessageBubble
                key={r.key}
                message={m}
                mine={m.senderId === meId}
                sender={users[m.senderId]}
                showSender={conv.kind === "group" && m.senderId !== meId && r.first}
                repliedTo={replied}
                repliedToName={replied ? (replied.senderId === meId ? "You" : nameOf(replied.senderId)) : undefined}
                firstInRun={r.first}
                lastInRun={r.last}
                highlighted={flashId === m.id}
                meId={meId}
                onOpenMenu={onOpenMenu}
                onJumpTo={jumpTo}
                onRetry={onRetry}
                onDiscard={onDiscard}
                onReact={onReact}
              />
            );
          })}
          {typers.length > 0 && (
            <div className="msg-row in" aria-live="polite">
              <div className="bubble in typing-bubble" aria-label={`${typers.map(nameOf).join(", ")} typing`}>
                <span className="typing-dots" aria-hidden="true"><span /><span /><span /></span>
              </div>
            </div>
          )}
        </div>
      </div>
      {newBelow > 0 && (
        <button type="button" className="to-bottom" onClick={() => scrollToBottom(true)}>
          <Icon name="arrow-down" size={16} /> {newBelow} new {newBelow === 1 ? "message" : "messages"}
        </button>
      )}
    </div>
  );
}
