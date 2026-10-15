import { useCallback, useEffect, useState } from "react";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { toast } from "@/components/common/Toast";
import { Icon } from "@/icons/Icon";
import { ConfirmDialog } from "@/components/common/Modal";
import { Link, navigate } from "@/lib/router";
import { session } from "@/services/session";
import { formatLastSeen } from "@/lib/utils";
import { services } from "@/services";
import { errorMessage } from "@/services/errors";
import { removeConversation, setFriend, upsertConversation } from "@/state/actions";
import type { GroupMember } from "@/services/types";
import { AddMembersDialog } from "./GroupDialogs";
import { useApp } from "@/state/store";
import type { Conversation } from "@/types/models";

export function ChatDetails({ conv }: { conv: Conversation }) {
  const peer = useApp((s) => (conv.peerId ? s.users[conv.peerId] : undefined));
  const users = useApp((s) => s.users);
  const friend = useApp((s) => s.friends.find((f) => f.user.id === conv.peerId));

  const meId = session.get()?.user.id ?? "";
  const [members, setMembers] = useState<GroupMember[] | null>(null);
  const [adding, setAdding] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [draftName, setDraftName] = useState(conv.title);
  const isGroup = conv.kind === "group";
  const myRole = members?.find((m) => m.userId === meId)?.role;
  const canManage = myRole === "owner" || myRole === "admin";

  const loadMembers = useCallback(async () => {
    try {
      setMembers(await services.chats.members(conv.id));
    } catch {
      setMembers([]);
    }
  }, [conv.id]);
  useEffect(() => {
    setMembers(null);
    if (isGroup) void loadMembers();
  }, [conv.id, isGroup, loadMembers]);
  useEffect(() => {
    if (!isGroup) return;
    const on = (ev: Event) => {
      const id = (ev as CustomEvent<string | undefined>).detail;
      if (!id || id === conv.id) void loadMembers();
    };
    window.addEventListener("kikbite:group-changed", on);
    return () => window.removeEventListener("kikbite:group-changed", on);
  }, [conv.id, isGroup, loadMembers]);
  useEffect(() => setDraftName(conv.title), [conv.title]);

  const rename = async () => {
    const next = draftName.trim();
    setRenaming(false);
    if (!next || next === conv.title) return setDraftName(conv.title);
    try {
      upsertConversation(await services.chats.renameGroup(conv.id, next));
      toast.success("Group renamed");
    } catch (e) {
      setDraftName(conv.title);
      toast.error(errorMessage(e, "Couldn't rename the group."));
    }
  };
  const removeFromGroup = async (userId: string) => {
    try {
      await services.chats.removeMember(conv.id, userId);
      if (userId === meId) {
        removeConversation(conv.id);
        navigate("/chats");
        toast.success("You left the group");
      } else {
        setMembers((m) => m && m.filter((x) => x.userId !== userId));
        toast.success("Removed from the group");
      }
    } catch (e) {
      toast.error(errorMessage(e, "Couldn't do that."));
    }
  };

  const patch = async (p: Partial<Pick<Conversation, "pinned" | "muted" | "archived">>, msg: string) => {
    const before = conv;
    upsertConversation({ ...conv, ...p });
    try {
      upsertConversation(await services.chats.update(conv.id, p));
      toast.success(msg);
    } catch (e) {
      upsertConversation(before);
      toast.error(errorMessage(e));
    }
  };
  const block = async () => {
    if (!peer) return;
    try {
      await services.friends.block(peer.id);
      setFriend({ user: peer, status: "blocked" });
      toast.success(`${peer.displayName} blocked`);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <div className="details">
      <div className="details-hero">
        <Avatar name={conv.title} id={conv.peerId ?? conv.id} src={conv.avatarUrl} size={88} group={conv.kind === "group"} presence={conv.kind === "direct" ? peer?.presence : undefined} tone={conv.avatarTone} />
        {isGroup && canManage && renaming ? (
          <input className="input" aria-label="Group name" value={draftName} maxLength={50} autoFocus onChange={(e: any) => setDraftName(e.target.value)} onBlur={() => void rename()} onKeyDown={(e: any) => { if (e.key === "Enter") void rename(); if (e.key === "Escape") { setDraftName(conv.title); setRenaming(false); } }} />
        ) : (
          <h2>{conv.title}</h2>
        )}
        {isGroup && canManage && !renaming && <Button variant="ghost" size="sm" onClick={() => setRenaming(true)}>Rename group</Button>}
        {conv.kind === "direct" && peer && <p className="field-hint">@{peer.username} · {formatLastSeen(peer)}</p>}
        {conv.kind === "group" && <p className="field-hint">{members ? members.length : conv.memberIds.length} members</p>}
        {peer?.bio && <p className="details-bio">{peer.bio}</p>}
      </div>
      <div className="details-actions">
        <Button variant="secondary" size="sm" onClick={() => void patch({ pinned: !conv.pinned }, conv.pinned ? "Unpinned" : "Pinned")}><Icon name="pin" size={16} /> {conv.pinned ? "Unpin" : "Pin"}</Button>
        <Button variant="secondary" size="sm" onClick={() => void patch({ muted: !conv.muted }, conv.muted ? "Unmuted" : "Muted")}><Icon name="bell" size={16} /> {conv.muted ? "Unmute" : "Mute"}</Button>
        <Button variant="secondary" size="sm" onClick={() => void patch({ archived: !conv.archived }, conv.archived ? "Unarchived" : "Archived")}><Icon name="download" size={16} /> {conv.archived ? "Unarchive" : "Archive"}</Button>
      </div>
      {isGroup && (
        <section aria-labelledby="mem-h" className="details-section">
          <h3 id="mem-h" className="panel-title">Members</h3>
          {canManage && <Button variant="secondary" size="sm" onClick={() => setAdding(true)}><Icon name="plus" size={16} /> Add people</Button>}
          <ul className="panel-list">
            {(members ?? []).map(({ userId, role }) => {
              const u = users[userId];
              if (!u) return null;
              return (
                <li key={userId} style={{ display: "flex", alignItems: "center" }}>
                  <div style={{ flex: 1, minWidth: 0 }}><Link to={`/profile/${userId}`} className="panel-row">
                    <Avatar name={u.displayName} id={userId} src={u.avatarUrl} size={34} presence={u.presence} />
                    <span className="truncate">{userId === meId ? "You" : u.displayName}</span>
                    {role !== "member" && <span className="field-hint">{role}</span>}
                  </Link></div>
                  {canManage && userId !== meId && role !== "owner" && <Button variant="ghost" size="sm" aria-label={`Remove ${u.displayName}`} onClick={() => void removeFromGroup(userId)}>Remove</Button>}
                </li>
              );
            })}
          </ul>
          <Button variant="danger" size="sm" onClick={() => setLeaving(true)}>Leave group</Button>
        </section>
      )}
      {adding && <AddMembersDialog convId={conv.id} exclude={(members ?? []).map((m) => m.userId)} onClose={() => setAdding(false)} onDone={() => void loadMembers()} />}
      {leaving && <ConfirmDialog title="Leave group?" message="You will stop receiving messages from this group." confirmLabel="Leave" danger onConfirm={() => void removeFromGroup(meId)} onClose={() => setLeaving(false)} />}
      {conv.kind === "direct" && peer && (
        <div className="details-actions col">
          <Link to={`/profile/${peer.id}`} className="btn btn-secondary btn-sm"><Icon name="user" size={16} /> View profile</Link>
          {friend?.status !== "blocked" && <Button variant="danger" size="sm" onClick={() => void block()}><Icon name="block" size={16} /> Block {peer.displayName.split(" ")[0]}</Button>}
        </div>
      )}
    </div>
  );
}
