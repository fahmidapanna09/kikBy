import { useState } from "react";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { Modal } from "@/components/common/Modal";
import { toast } from "@/components/common/Toast";
import { Icon } from "@/icons/Icon";
import { cn } from "@/lib/utils";
import { errorMessage } from "@/services/errors";
import { forwardMessage } from "@/state/messages";
import { useApp } from "@/state/store";
import type { Message } from "@/types/models";

export function ForwardDialog({ message, onClose }: { message: Message; onClose: () => void }) {
  const convs = useApp((s) => s.conversations);
  const [picked, setPicked] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [q, setQ] = useState("");
  const list = convs.filter((c) => c.title.toLowerCase().includes(q.trim().toLowerCase()));
  const toggle = (id: string) => setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id]));

  const send = async () => {
    setBusy(true);
    try {
      await forwardMessage(message, picked);
      toast.success(`Forwarded to ${picked.length} ${picked.length === 1 ? "chat" : "chats"}`);
      onClose();
    } catch (e) {
      toast.error(errorMessage(e, "Couldn't forward the message."));
      setBusy(false);
    }
  };

  return (
    <Modal
      title="Forward to…"
      onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button disabled={picked.length === 0} loading={busy} onClick={() => void send()}>Send{picked.length ? ` (${picked.length})` : ""}</Button></>}
    >
      <div className="input-wrap" style={{ marginBottom: 8 }}>
        <Icon name="search" />
        <input className="input" placeholder="Search chats" aria-label="Search chats" value={q} onChange={(e: any) => setQ(e.target.value)} data-autofocus />
      </div>
      <ul className="pick-list" aria-label="Chats">
        {list.map((c) => {
          const on = picked.includes(c.id);
          return (
            <li key={c.id}>
              <button type="button" className="pick-row" role="checkbox" aria-checked={on} onClick={() => toggle(c.id)}>
                <Avatar name={c.title} id={c.peerId ?? c.id} size={36} group={c.kind === "group"} />
                <span className="truncate" style={{ flex: 1, textAlign: "left", color: "var(--text)" }}>{c.title}</span>
                <span className={cn("checkbox", on && "on")}>{on && <Icon name="check" size={12} />}</span>
              </button>
            </li>
          );
        })}
        {list.length === 0 && <li className="field-hint" style={{ padding: 12 }}>No chats match “{q}”.</li>}
      </ul>
    </Modal>
  );
}
