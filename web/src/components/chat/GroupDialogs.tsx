import { useState } from "react";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { Modal } from "@/components/common/Modal";
import { toast } from "@/components/common/Toast";
import { Icon } from "@/icons/Icon";
import { navigate } from "@/lib/router";
import { cn } from "@/lib/utils";
import { services } from "@/services";
import { errorMessage } from "@/services/errors";
import { upsertConversation } from "@/state/actions";
import { useApp } from "@/state/store";
import type { ID, User } from "@/types/models";

const MAX_NAME = 50;

/** Friends the user can pick from (accepted friends only), minus anyone already in `exclude`. */
function FriendPicker({ exclude, picked, onToggle }: { exclude: ID[]; picked: ID[]; onToggle: (id: ID) => void }) {
  const friends = useApp((s) => s.friends);
  const [q, setQ] = useState("");
  const list = friends
    .filter((f) => f.status === "friends" && !exclude.includes(f.user.id))
    .map((f) => f.user)
    .filter((u: User) => !q.trim() || u.displayName.toLowerCase().includes(q.trim().toLowerCase()) || u.username.toLowerCase().includes(q.trim().toLowerCase()));
  return (
    <>
      <div className="input-wrap" style={{ marginBottom: 8 }}>
        <Icon name="search" />
        <input className="input" placeholder="Search friends" aria-label="Search friends" value={q} onChange={(e: any) => setQ(e.target.value)} />
      </div>
      <ul className="pick-list" aria-label="Friends">
        {list.map((u) => {
          const on = picked.includes(u.id);
          return (
            <li key={u.id}>
              <button type="button" className="pick-row" role="checkbox" aria-checked={on} aria-label={u.displayName} onClick={() => onToggle(u.id)}>
                <Avatar name={u.displayName} id={u.id} src={u.avatarUrl} size={36} />
                <span className="truncate" style={{ flex: 1, textAlign: "left", color: "var(--text)" }}>{u.displayName}</span>
                <span className={cn("checkbox", on && "on")}>{on && <Icon name="check" size={12} />}</span>
              </button>
            </li>
          );
        })}
        {list.length === 0 && <li className="field-hint" style={{ padding: 12 }}>{q ? `No friends match “${q}”.` : "No friends to add yet."}</li>}
      </ul>
    </>
  );
}

export function NewGroupDialog({ onClose }: { onClose: () => void }) {
  const [name, setName] = useState("");
  const [picked, setPicked] = useState<ID[]>([]);
  const [busy, setBusy] = useState(false);
  const toggle = (id: ID) => setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id]));
  const ok = name.trim().length >= 1 && picked.length >= 1;

  const create = async () => {
    setBusy(true);
    try {
      const c = await services.chats.createGroup(name, picked);
      upsertConversation(c);
      onClose();
      navigate(`/chats/${c.id}`);
    } catch (e) {
      toast.error(errorMessage(e, "Couldn't create the group."));
      setBusy(false);
    }
  };

  return (
    <Modal
      title="New group"
      onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button disabled={!ok} loading={busy} onClick={() => void create()}>Create{picked.length ? ` (${picked.length})` : ""}</Button></>}
    >
      <label className="field-label" htmlFor="grp-name">Group name</label>
      <input id="grp-name" className="input" style={{ marginBottom: 12 }} value={name} maxLength={MAX_NAME} placeholder="e.g. Weekend plans" onChange={(e: any) => setName(e.target.value)} data-autofocus />
      <p className="field-hint" style={{ marginBottom: 6 }}>Add friends</p>
      <FriendPicker exclude={[]} picked={picked} onToggle={toggle} />
    </Modal>
  );
}

export function AddMembersDialog({ convId, exclude, onClose, onDone }: { convId: ID; exclude: ID[]; onClose: () => void; onDone: () => void }) {
  const [picked, setPicked] = useState<ID[]>([]);
  const [busy, setBusy] = useState(false);
  const toggle = (id: ID) => setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id]));
  const add = async () => {
    setBusy(true);
    try {
      await services.chats.addMembers(convId, picked);
      toast.success(`Added ${picked.length} ${picked.length === 1 ? "person" : "people"}`);
      onDone();
      onClose();
    } catch (e) {
      toast.error(errorMessage(e, "Couldn't add people."));
      setBusy(false);
    }
  };
  return (
    <Modal title="Add people" onClose={onClose} footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button disabled={!picked.length} loading={busy} onClick={() => void add()}>Add{picked.length ? ` (${picked.length})` : ""}</Button></>}>
      <FriendPicker exclude={exclude} picked={picked} onToggle={toggle} />
    </Modal>
  );
}
