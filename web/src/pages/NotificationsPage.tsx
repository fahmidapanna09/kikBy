import { useState } from "react";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { EmptyState, ErrorState } from "@/components/common/EmptyState";
import { ListSkeleton } from "@/components/common/Skeleton";
import { toast } from "@/components/common/Toast";
import { Icon } from "@/icons/Icon";
import type { IconName } from "@/icons/Icon";
import { navigate } from "@/lib/router";
import { cn, formatListTime } from "@/lib/utils";
import { services } from "@/services";
import { errorMessage } from "@/services/errors";
import { loadNotifications } from "@/state/actions";
import { store, useApp } from "@/state/store";
import type { AppNotification } from "@/types/models";

const ICON: Record<AppNotification["kind"], IconName> = { message: "chat", friend_request: "user-plus", friend_accept: "users", mention: "at", system: "info" };

function target(n: AppNotification): string | null {
  if (n.conversationId) return `/chats/${n.conversationId}`;
  if (n.kind === "friend_request") return "/friends";
  if (n.actorId) return `/profile/${n.actorId}`;
  return null;
}

const mark = (ids: string[], read: boolean) => store.set((s) => ({ notifications: s.notifications.map((n) => (ids.includes(n.id) ? { ...n, read } : n)) }));

export default function NotificationsPage() {
  const status = useApp((s) => s.notifStatus);
  const error = useApp((s) => s.notifError);
  const list = useApp((s) => s.notifications);
  const users = useApp((s) => s.users);
  const [tab, setTab] = useState<"all" | "unread">("all");
  const unread = list.filter((n) => !n.read).length;
  const shown = tab === "unread" ? list.filter((n) => !n.read) : list;

  const open = async (n: AppNotification) => {
    if (!n.read) {
      mark([n.id], true);
      services.notifications.markRead(n.id).catch(() => mark([n.id], false));
    }
    const to = target(n);
    if (to) navigate(to);
  };
  const markAll = async () => {
    const ids = list.filter((n) => !n.read).map((n) => n.id);
    mark(ids, true);
    try {
      await services.notifications.markAllRead();
      toast.success("All caught up");
    } catch (e) {
      mark(ids, false);
      toast.error(errorMessage(e));
    }
  };

  return (
    <div className="page">
      <div className="page-inner">
        <div className="page-head-row">
          <h2 className="page-title">Notifications</h2>
          <Button size="sm" variant="secondary" disabled={unread === 0} onClick={() => void markAll()}><Icon name="check-double" size={16} /> Mark all as read</Button>
        </div>
        <div className="tabs" role="tablist" aria-label="Filter notifications">
          <button type="button" role="tab" className="tab" aria-selected={tab === "all"} onClick={() => setTab("all")}>All</button>
          <button type="button" role="tab" className="tab" aria-selected={tab === "unread"} onClick={() => setTab("unread")}>Unread{unread ? ` (${unread})` : ""}</button>
        </div>
        <section className="card" aria-live="polite">
          {status === "idle" || status === "loading" ? <ListSkeleton rows={5} /> :
           status === "error" ? <ErrorState message={error} onRetry={() => void loadNotifications()} /> :
           shown.length === 0 ? <EmptyState icon="bell" title={tab === "unread" ? "You're all caught up" : "No notifications yet"} message={tab === "unread" ? "No unread notifications." : "Messages, requests and mentions will appear here."} /> :
           <ul>{shown.map((n) => {
             const actor = n.actorId ? users[n.actorId] : undefined;
             return (
               <li key={n.id}>
                 <button type="button" className={cn("notif", !n.read && "unread")} onClick={() => void open(n)} aria-label={`${n.read ? "" : "Unread. "}${n.title} ${n.body}`}>
                   {actor ? <Avatar name={actor.displayName} id={actor.id} src={actor.avatarUrl} size={44} /> : <span className="notif-icon"><Icon name={ICON[n.kind]} /></span>}
                   <span className="notif-text">
                     <span className="notif-title">{n.title}</span>
                     <span className="notif-body">{n.body}</span>
                   </span>
                   <span className="notif-side">
                     <time dateTime={n.createdAt}>{formatListTime(n.createdAt)}</time>
                     {!n.read && <span className="unread-dot" aria-hidden="true" />}
                   </span>
                 </button>
               </li>
             );
           })}</ul>}
        </section>
      </div>
    </div>
  );
}
