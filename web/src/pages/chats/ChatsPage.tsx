import { Button } from "@/components/common/Button";
import { EmptyState } from "@/components/common/EmptyState";
import { Skeleton } from "@/components/common/Skeleton";
import { ChatList } from "@/components/chat/ChatList";
import { navigate } from "@/lib/router";
import { useSession } from "@/services";
import { useApp } from "@/state/store";
import { ChatView } from "./ChatView";

/** /chats (list) and /chats/:id (list + conversation). On mobile only one pane shows at a time. */
export default function ChatsPage({ id }: { id?: string }) {
  const me = useSession()?.user;
  const status = useApp((s) => s.convStatus);
  const conv = useApp((s) => (id ? s.conversations.find((c) => c.id === id) : undefined));
  if (!me) return null;

  let pane;
  if (!id) pane = <EmptyState icon="chat" title="Select a conversation" message="Pick a chat from the list, or find someone new to talk to." actions={<Button size="sm" onClick={() => navigate("/search")}>Find people</Button>} />;
  else if (conv) pane = <ChatView key={conv.id} conv={conv} meId={me.id} />;
  else if (status === "idle" || status === "loading") pane = <div style={{ padding: 24, display: "grid", gap: 12 }} role="status" aria-label="Loading"><Skeleton h={48} /><Skeleton h={300} /></div>;
  else pane = <EmptyState icon="alert" title="Conversation not found" message="It may have been removed, or you no longer have access." actions={<Button size="sm" onClick={() => navigate("/chats")}>Back to chats</Button>} />;

  return (
    <div className="chats" data-open={!!id}>
      <div className="chats-list-col"><ChatList meId={me.id} activeId={id} /></div>
      <div className="chats-main-col">{pane}</div>
    </div>
  );
}
