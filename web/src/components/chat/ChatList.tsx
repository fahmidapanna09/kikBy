import { useMemo, useState } from "react";
import { IconButton } from "@/components/common/Button";
import { Menu } from "@/components/common/Dropdown";
import type { MenuEntry } from "@/components/common/Dropdown";
import { EmptyState, ErrorState } from "@/components/common/EmptyState";
import { ListSkeleton } from "@/components/common/Skeleton";
import { toast } from "@/components/common/Toast";
import { Icon } from "@/icons/Icon";
import { navigate } from "@/lib/router";
import { cn } from "@/lib/utils";
import { services } from "@/services";
import { errorMessage } from "@/services/errors";
import { loadConversations, upsertConversation } from "@/state/actions";
import { useApp } from "@/state/store";
import type { Conversation } from "@/types/models";
import { ConversationItem } from "./ConversationItem";
import { NewGroupDialog } from "./GroupDialogs";

type Filter = "all" | "unread" | "groups" | "archived";
const FILTERS: Array<[Filter, string]> = [["all", "All"], ["unread", "Unread"], ["groups", "Groups"], ["archived", "Archived"]];

export function ChatList({ meId, activeId }: { meId: string; activeId?: string }) {
  const status = useApp((s) => s.convStatus);
  const error = useApp((s) => s.convError);
  const convs = useApp((s) => s.conversations);
  const [q, setQ] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [newGroup, setNewGroup] = useState(false);
  const [menu, setMenu] = useState<{ c: Conversation; x: number; y: number } | null>(null);

  const list = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return convs
      .filter((c) => (filter === "archived" ? c.archived : !c.archived))
      .filter((c) => (filter === "unread" ? c.unreadCount > 0 : filter === "groups" ? c.kind === "group" : true))
      .filter((c) => !needle || c.title.toLowerCase().includes(needle) || (c.lastMessage?.text ?? "").toLowerCase().includes(needle))
      .sort((a, b) => Number(b.pinned) - Number(a.pinned) || +new Date(b.updatedAt) - +new Date(a.updatedAt));
  }, [convs, q, filter]);

  const patch = async (c: Conversation, p: Partial<Pick<Conversation, "pinned" | "muted" | "archived">>) => {
    upsertConversation({ ...c, ...p });
    try {
      upsertConversation(await services.chats.update(c.id, p));
    } catch (e) {
      upsertConversation(c);
      toast.error(errorMessage(e));
    }
  };
  const items = (c: Conversation): MenuEntry[] => [
    { label: c.pinned ? "Unpin" : "Pin to top", icon: "pin", onSelect: () => void patch(c, { pinned: !c.pinned }) },
    { label: c.muted ? "Unmute" : "Mute", icon: "bell", onSelect: () => void patch(c, { muted: !c.muted }) },
    { label: c.archived ? "Unarchive" : "Archive", icon: "download", onSelect: () => void patch(c, { archived: !c.archived }) },
  ];

  return (
    <section className="chat-list" aria-label="Conversations">
      <div className="chat-list-head">
        <h2>Chats</h2>
        <div style={{ display: "flex", gap: 4 }}>
          <IconButton label="New group" onClick={() => setNewGroup(true)}><Icon name="users" /></IconButton>
          <IconButton label="New chat" onClick={() => navigate("/search")}><Icon name="plus" /></IconButton>
        </div>
      </div>
      <div className="chat-list-search">
        <div className="input-wrap" style={{ minHeight: 40 }}>
          <Icon name="search" />
          <input className="input" type="search" placeholder="Search chats" aria-label="Search chats" value={q} onChange={(e: any) => setQ(e.target.value)} style={{ padding: "8px 0" }} />
        </div>
      </div>
      <div className="chips" role="tablist" aria-label="Filter chats">
        {FILTERS.map(([k, label]) => (
          <button key={k} type="button" role="tab" aria-selected={filter === k} className={cn("chip", filter === k && "on")} onClick={() => setFilter(k)}>{label}</button>
        ))}
      </div>
      <div className="chat-list-body">
        {status === "idle" || status === "loading" ? (
          <ListSkeleton />
        ) : status === "error" ? (
          <ErrorState message={error} onRetry={() => void loadConversations()} />
        ) : list.length === 0 ? (
          q || filter !== "all" ? (
            <EmptyState icon="search" title="No matching chats" message={q ? `Nothing found for “${q}”.` : "Nothing here yet."} />
          ) : (
            <EmptyState icon="chat" title="No conversations yet" message="Find people by username and start chatting." />
          )
        ) : (
          <ul>
            {list.map((c) => (
              <li key={c.id} onContextMenu={(e: any) => { e.preventDefault(); setMenu({ c, x: e.clientX, y: e.clientY }); }}>
                <ConversationItem conv={c} meId={meId} active={c.id === activeId} />
              </li>
            ))}
          </ul>
        )}
      </div>
      {newGroup && <NewGroupDialog onClose={() => setNewGroup(false)} />}
      {menu && <Menu x={menu.x} y={menu.y} items={items(menu.c)} onClose={() => setMenu(null)} />}
    </section>
  );
}
