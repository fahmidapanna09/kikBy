import { memo } from "react";
import { Avatar } from "@/components/common/Avatar";
import { Icon } from "@/icons/Icon";
import { Link } from "@/lib/router";
import { cn, formatListTime } from "@/lib/utils";
import { useApp } from "@/state/store";
import type { Conversation } from "@/types/models";

interface Props {
  conv: Conversation;
  meId: string;
  active?: boolean;
  compact?: boolean;
}

export const ConversationItem = memo(function ConversationItem({ conv, meId, active, compact }: Props) {
  const peer = useApp((s) => (conv.peerId ? s.users[conv.peerId] : undefined));
  const typing = useApp((s) => s.typing[conv.id]);
  const sender = useApp((s) => (conv.kind === "group" && conv.lastMessage ? s.users[conv.lastMessage.senderId] : undefined));
  const last = conv.lastMessage;
  const preview = typing?.length
    ? "typing…"
    : last
      ? `${last.senderId === meId ? "You: " : sender ? `${sender.displayName.split(" ")[0]}: ` : ""}${last.text || "Attachment"}`
      : "No messages yet";
  const unread = conv.unreadCount > 0;
  return (
    <Link
      to={`/chats/${conv.id}`}
      className={cn("conv-item", active && "active", compact && "compact")}
      aria-current={active ? "page" : undefined}
      aria-label={`${conv.title}${unread ? `, ${conv.unreadCount} unread` : ""}`}
    >
      <Avatar name={conv.title} id={conv.peerId ?? conv.id} src={conv.avatarUrl} size={48} group={conv.kind === "group"} presence={conv.kind === "direct" ? peer?.presence : undefined} tone={conv.avatarTone} />
      <span className="conv-body">
        <span className="conv-top">
          <span className={cn("conv-name truncate", unread && "unread")}>{conv.title}</span>
          {last && <time className={cn("conv-time", unread && "unread")} dateTime={last.createdAt}>{formatListTime(last.createdAt)}</time>}
        </span>
        <span className="conv-bottom">
          <span className={cn("conv-preview truncate", typing?.length ? "typing" : unread && "unread")}>{preview}</span>
          {conv.muted && <Icon name="block" size={14} className="conv-flag" label="Muted" />}
          {conv.pinned && <Icon name="pin" size={14} className="conv-flag" label="Pinned" />}
          {unread && <span className={cn("badge", conv.muted && "muted")}>{conv.unreadCount > 99 ? "99+" : conv.unreadCount}</span>}
        </span>
      </span>
    </Link>
  );
});
