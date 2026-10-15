import { useCallback, useEffect, useState } from "react";
import { callsAvailable, joinLive, refreshLive } from "@/calls/callClient";
import { useCalls } from "@/calls/callStore";
import { Icon } from "@/icons/Icon";
import { ConfirmDialog, Drawer } from "@/components/common/Modal";
import { Menu } from "@/components/common/Dropdown";
import type { MenuEntry } from "@/components/common/Dropdown";
import { toast } from "@/components/common/Toast";
import { ChatDetails } from "@/components/chat/ChatDetails";
import { ChatHeader } from "@/components/chat/ChatHeader";
import { Composer } from "@/components/chat/Composer";
import { ForwardDialog } from "@/components/chat/ForwardDialog";
import { MessageList } from "@/components/chat/MessageList";
import { useIsDesktop } from "@/hooks/useMedia";
import { errorMessage } from "@/services/errors";
import { closeThread, deleteMessage, discardFailed, openThread, reactToMessage, retryMessage } from "@/state/messages";
import { useApp } from "@/state/store";
import type { Conversation, Message } from "@/types/models";

const QUICK = ["👍", "❤️", "😂", "😮", "😢", "🙏"];

export function ChatView({ conv, meId }: { conv: Conversation; meId: string }) {
  const users = useApp((s) => s.users);
  const [menu, setMenu] = useState<{ m: Message; x: number; y: number } | null>(null);
  const [replyTo, setReplyTo] = useState<Message | null>(null);
  const [editing, setEditing] = useState<Message | null>(null);
  const [forwarding, setForwarding] = useState<Message | null>(null);
  const [deleting, setDeleting] = useState<Message | null>(null);
  const [details, setDetails] = useState(false);
  const desktop = useIsDesktop();
  const live = useCalls((s) => s.live[conv.id]);
  const inThisCall = useCalls((s) => s.active?.conversationId === conv.id);
  const busy = useCalls((s) => !!s.active);

  useEffect(() => {
    void openThread(conv.id);
    setReplyTo(null);
    setEditing(null);
    setMenu(null);
    return () => closeThread(conv.id);
  }, [conv.id]);

  useEffect(() => {
    if (callsAvailable()) void refreshLive(conv.id);
  }, [conv.id]);

  // Refresh read state when the tab becomes visible again while this chat is open.
  useEffect(() => {
    const onVis = () => document.visibilityState === "visible" && void openThread(conv.id);
    document.addEventListener("visibilitychange", onVis);
    return () => document.removeEventListener("visibilitychange", onVis);
  }, [conv.id]);

  const react = useCallback((m: Message, emoji: string) => void reactToMessage(m, emoji).catch((e) => toast.error(errorMessage(e))), []);
  const openMenu = useCallback((m: Message, x: number, y: number) => setMenu({ m, x, y }), []);
  const nameOf = (id: string) => (id === meId ? "You" : users[id]?.displayName ?? "Someone");

  const items = (m: Message): MenuEntry[] => {
    const mine = m.senderId === meId;
    return [
      { label: "Reply", icon: "reply", onSelect: () => { setEditing(null); setReplyTo(m); } },
      { label: "Copy text", icon: "copy", hidden: !m.text, onSelect: () => void navigator.clipboard?.writeText(m.text).then(() => toast.success("Copied"), () => toast.error("Couldn't copy")) },
      { label: "Forward", icon: "forward", onSelect: () => setForwarding(m) },
      { label: "Edit", icon: "edit", hidden: !mine || !m.text, separatorBefore: true, onSelect: () => { setReplyTo(null); setEditing(m); } },
      { label: "Delete", icon: "trash", danger: true, hidden: !mine, onSelect: () => setDeleting(m) },
    ];
  };

  return (
    <div className="chat-view">
      <div className="chat-col">
        <ChatHeader conv={conv} detailsOpen={details} onToggleDetails={() => setDetails((d) => !d)} />
        {live && !inThisCall && (
          <div className="call-banner" role="status">
            <Icon name={live.kind === "video" ? "video" : "phone"} size={16} />
            <span>{live.kind === "video" ? "Video" : "Voice"} call in progress</span>
            <button type="button" className="btn btn-primary btn-sm" disabled={busy} onClick={() => joinLive(conv.id)}>Join</button>
            {live.kind === "video" && <button type="button" className="btn btn-sm" disabled={busy} onClick={() => joinLive(conv.id, true)}>Join with audio only</button>}
          </div>
        )}
        <MessageList conv={conv} meId={meId} onOpenMenu={openMenu} onRetry={retryMessage} onDiscard={discardFailed} onReact={react} />
        <Composer conv={conv} replyTo={replyTo} replyToName={replyTo ? nameOf(replyTo.senderId) : undefined} editing={editing} onCancelContext={() => { setReplyTo(null); setEditing(null); }} />
      </div>
      {details && desktop && <aside className="chat-details-pane" aria-label="Conversation details"><ChatDetails conv={conv} /></aside>}
      {details && !desktop && <Drawer title="Details" onClose={() => setDetails(false)}><ChatDetails conv={conv} /></Drawer>}
      {menu && (
        <Menu
          x={menu.x}
          y={menu.y}
          items={items(menu.m)}
          onClose={() => setMenu(null)}
          header={
            <div className="reaction-bar" role="group" aria-label="React">
              {QUICK.map((e) => <button key={e} type="button" aria-label={`React ${e}`} onClick={() => { const m = menu.m; setMenu(null); react(m, e); }}>{e}</button>)}
            </div>
          }
        />
      )}
      {forwarding && <ForwardDialog message={forwarding} onClose={() => setForwarding(null)} />}
      {deleting && <ConfirmDialog title="Delete message?" message="This message will be removed for everyone in this chat." confirmLabel="Delete" danger onClose={() => setDeleting(null)} onConfirm={() => deleteMessage(deleting).catch((e) => toast.error(errorMessage(e)))} />}
    </div>
  );
}
