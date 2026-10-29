import { useState } from "react";
import { Button } from "./Button";
import { ConfirmDialog } from "./Modal";
import { Icon } from "@/icons/Icon";
import { friendActions } from "@/state/friends";
import { useApp } from "@/state/store";
import type { User } from "@/types/models";

/** Follow model: Follow / Following (tap to unfollow) / Follow back + Ignore / Message + Unfollow (friends = follow each other) / Unblock. */
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
            <Button size={size} variant="ghost" aria-label={`Unfollow ${user.displayName}`} onClick={() => setConfirm("remove")}><Icon name="user-minus" size={16} />{!compact && " Unfollow"}</Button>
          </>
        );
      case "incoming":
        return (
          <>
            <Button size={size} onClick={() => void friendActions.accept(user)}>Follow back</Button>
            <Button size={size} variant="secondary" aria-label={`Ignore ${user.displayName}`} onClick={() => void friendActions.reject(user)}>Ignore</Button>
          </>
        );
      case "outgoing":
        return <Button size={size} variant="secondary" aria-label={`Following ${user.displayName}, tap to unfollow`} onClick={() => void friendActions.cancel(user)}>Following</Button>;
      case "blocked":
        return <Button size={size} variant="secondary" onClick={() => void friendActions.unblock(user)}>Unblock</Button>;
      default:
        return (
          <>
            <Button size={size} onClick={() => void friendActions.add(user)}><Icon name="user-plus" size={16} /> Follow</Button>
            <Button size={size} variant="ghost" aria-label={`Message ${user.displayName}`} onClick={() => void friendActions.message(user)}><Icon name="chat" size={16} /></Button>
          </>
        );
    }
  })();
  return (
    <>
      {buttons}
      {confirm === "remove" && <ConfirmDialog title={`Unfollow ${user.displayName}?`} message={`You will stop being friends. ${user.displayName} will still follow you, and your chat history stays.`} confirmLabel="Unfollow" danger onConfirm={() => friendActions.remove(user)} onClose={() => setConfirm(null)} />}
      {confirm === "block" && <ConfirmDialog title={`Block ${user.displayName}?`} message="They won't be able to message you or see your profile details." confirmLabel="Block" danger onConfirm={() => friendActions.block(user)} onClose={() => setConfirm(null)} />}
    </>
  );
}
