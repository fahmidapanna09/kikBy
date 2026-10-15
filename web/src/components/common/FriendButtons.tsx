import { useState } from "react";
import { Button } from "./Button";
import { ConfirmDialog } from "./Modal";
import { Icon } from "@/icons/Icon";
import { friendActions } from "@/state/friends";
import { useApp } from "@/state/store";
import type { User } from "@/types/models";

/** Context-aware buttons for any user: add / pending / accept+decline / message+remove / unblock. */
export function FriendButtons({ user, compact }: { user: User; compact?: boolean }) {
  const status = useApp((s) => s.friends.find((f) => f.user.id === user.id)?.status ?? "none");
  const [confirm, setConfirm] = useState<"remove" | "block" | null>(null);
  const size = "sm" as const;
  const buttons = (() => {
    switch (status) {
      case "friends":
        return (
          <>
            <Button size={size} aria-label={`Message ${user.displayName}`} onClick={() => void friendActions.message(user)}><Icon name="chat" size={16} />{!compact && " Message"}</Button>
            <Button size={size} variant="ghost" aria-label={`Remove ${user.displayName}`} onClick={() => setConfirm("remove")}><Icon name="user-minus" size={16} />{!compact && " Remove"}</Button>
          </>
        );
      case "incoming":
        return (
          <>
            <Button size={size} onClick={() => void friendActions.accept(user)}>Accept</Button>
            <Button size={size} variant="secondary" onClick={() => void friendActions.reject(user)}>Decline</Button>
          </>
        );
      case "outgoing":
        return <Button size={size} variant="secondary" onClick={() => void friendActions.cancel(user)}>Cancel request</Button>;
      case "blocked":
        return <Button size={size} variant="secondary" onClick={() => void friendActions.unblock(user)}>Unblock</Button>;
      default:
        return (
          <>
            <Button size={size} onClick={() => void friendActions.add(user)}><Icon name="user-plus" size={16} /> Add friend</Button>
            <Button size={size} variant="ghost" aria-label={`Message ${user.displayName}`} onClick={() => void friendActions.message(user)}><Icon name="chat" size={16} /></Button>
          </>
        );
    }
  })();
  return (
    <>
      {buttons}
      {confirm === "remove" && <ConfirmDialog title="Remove friend?" message={`${user.displayName} will be removed from your friends. Your chat history stays.`} confirmLabel="Remove" danger onConfirm={() => friendActions.remove(user)} onClose={() => setConfirm(null)} />}
      {confirm === "block" && <ConfirmDialog title={`Block ${user.displayName}?`} message="They won't be able to message you or see your profile details." confirmLabel="Block" danger onConfirm={() => friendActions.block(user)} onClose={() => setConfirm(null)} />}
    </>
  );
}
