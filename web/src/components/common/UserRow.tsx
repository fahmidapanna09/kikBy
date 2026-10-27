import type { ReactNode } from "react";
import { Avatar } from "./Avatar";
import { Link } from "@/lib/router";
import { formatLastSeen } from "@/lib/utils";
import { useApp } from "@/state/store";
import type { User } from "@/types/models";

/** Avatar + name + @username (+ note) with an actions slot. Live presence comes from the store cache. */
export function UserRow({ user, note, actions }: { user: User; note?: string; actions?: ReactNode }) {
  const live = useApp((s) => s.users[user.id]);
  const u = live ?? user;
  // "online" / "active 5 min ago" — only shown when the server told us (friends and chat partners), never "offline" noise.
  const seen = u.presence === "online" || u.lastSeenAt ? formatLastSeen(u) : "";
  return (
    <div className="user-row">
      <Link to={`/profile/${u.id}`} className="user-row-main" aria-label={`${u.displayName}, view profile`}>
        <Avatar name={u.displayName} id={u.id} src={u.avatarUrl} size={44} presence={u.presence} />
        <span className="user-row-text">
          <span className="user-row-name truncate">{u.displayName}</span>
          <span className="user-row-sub truncate">@{u.username}{note ? ` · ${note}` : ""}{seen ? ` · ${seen}` : ""}</span>
        </span>
      </Link>
      {actions && <div className="user-row-actions">{actions}</div>}
    </div>
  );
}
