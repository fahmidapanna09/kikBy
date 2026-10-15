import { useMemo, useState } from "react";
import { Button } from "@/components/common/Button";
import { EmptyState, ErrorState } from "@/components/common/EmptyState";
import { FriendButtons } from "@/components/common/FriendButtons";
import { Icon } from "@/icons/Icon";
import { ListSkeleton } from "@/components/common/Skeleton";
import { UserRow } from "@/components/common/UserRow";
import { navigate } from "@/lib/router";
import { cn } from "@/lib/utils";
import { loadFriends } from "@/state/actions";
import { useApp } from "@/state/store";
import type { Friend } from "@/types/models";

type Tab = "friends" | "requests" | "blocked";

export default function FriendsPage() {
  const status = useApp((s) => s.friendsStatus);
  const error = useApp((s) => s.friendsError);
  const friends = useApp((s) => s.friends);
  const [tab, setTab] = useState<Tab>("friends");
  const [q, setQ] = useState("");

  const match = (f: Friend) => !q.trim() || f.user.displayName.toLowerCase().includes(q.trim().toLowerCase()) || f.user.username.includes(q.trim().toLowerCase().replace(/^@/, ""));
  const groups = useMemo(() => ({
    friends: friends.filter((f) => f.status === "friends" && match(f)),
    incoming: friends.filter((f) => f.status === "incoming" && match(f)),
    outgoing: friends.filter((f) => f.status === "outgoing" && match(f)),
    blocked: friends.filter((f) => f.status === "blocked" && match(f)),
  }), [friends, q]); // eslint-disable-line react-hooks/exhaustive-deps
  const reqCount = friends.filter((f) => f.status === "incoming").length;

  const tabs: Array<[Tab, string, number?]> = [["friends", "Friends"], ["requests", "Requests", reqCount], ["blocked", "Blocked"]];

  const list = (items: Friend[], note: (f: Friend) => string | undefined) => (
    <ul className="user-list">{items.map((f) => <li key={f.user.id}><UserRow user={f.user} note={note(f)} actions={<FriendButtons user={f.user} compact />} /></li>)}</ul>
  );

  let body;
  if (status === "idle" || status === "loading") body = <ListSkeleton rows={6} />;
  else if (status === "error") body = <ErrorState message={error} onRetry={() => void loadFriends()} />;
  else if (tab === "friends") {
    body = groups.friends.length ? list(groups.friends, (f) => (f.mutualCount ? `${f.mutualCount} mutual` : undefined)) : q ? <EmptyState icon="search" title="No friends match" message={`Nothing found for “${q}”.`} /> : <EmptyState icon="users" title="No friends yet" message="Search by username to find people you know." actions={<Button size="sm" onClick={() => navigate("/search")}>Find people</Button>} />;
  } else if (tab === "requests") {
    body = groups.incoming.length + groups.outgoing.length === 0 ? <EmptyState icon="user-plus" title="No pending requests" message="Friend requests you send or receive show up here." /> : (
      <>
        {groups.incoming.length > 0 && <><h3 className="list-heading">Received · {groups.incoming.length}</h3>{list(groups.incoming, (f) => (f.mutualCount ? `${f.mutualCount} mutual` : undefined))}</>}
        {groups.outgoing.length > 0 && <><h3 className="list-heading">Sent · {groups.outgoing.length}</h3>{list(groups.outgoing, () => "pending")}</>}
      </>
    );
  } else {
    body = groups.blocked.length ? list(groups.blocked, () => "blocked") : <EmptyState icon="block" title="No blocked users" message="People you block can't message you." />;
  }

  return (
    <div className="page">
      <div className="page-inner">
        <h2 className="page-title">Friends</h2>
        <div className="tabs" role="tablist" aria-label="Friends sections">
          {tabs.map(([k, label, n]) => (
            <button key={k} type="button" role="tab" className="tab" aria-selected={tab === k} onClick={() => setTab(k)}>
              {label}{n ? <span className={cn("badge")} style={{ marginLeft: 6 }}>{n}</span> : null}
            </button>
          ))}
        </div>
        <div className="input-wrap">
          <Icon name="search" />
          <input className="input" type="search" placeholder="Filter by name or @username" aria-label="Filter friends" value={q} onChange={(e: any) => setQ(e.target.value)} />
        </div>
        <section className="card" aria-live="polite">{body}</section>
      </div>
    </div>
  );
}
