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

type Tab = "friends" | "followers" | "following" | "blocked";

export default function FriendsPage() {
  const status = useApp((s) => s.friendsStatus);
  const error = useApp((s) => s.friendsError);
  const friends = useApp((s) => s.friends);
  const [tab, setTab] = useState<Tab>("friends");
  const [q, setQ] = useState("");

  const match = (f: Friend) => !q.trim() || f.user.displayName.toLowerCase().includes(q.trim().toLowerCase()) || f.user.username.includes(q.trim().toLowerCase().replace(/^@/, ""));
  // friends = follow each other; incoming = follows me (I can follow back); outgoing = I follow them.
  const groups = useMemo(() => ({
    friends: friends.filter((f) => f.status === "friends" && match(f)),
    incoming: friends.filter((f) => f.status === "incoming" && match(f)),
    outgoing: friends.filter((f) => f.status === "outgoing" && match(f)),
    blocked: friends.filter((f) => f.status === "blocked" && match(f)),
  }), [friends, q]); // eslint-disable-line react-hooks/exhaustive-deps
  const newFollowers = friends.filter((f) => f.status === "incoming").length;
  const followerTotal = friends.filter((f) => f.status === "incoming" || f.status === "friends").length;
  const followingTotal = friends.filter((f) => f.status === "outgoing" || f.status === "friends").length;

  const tabs: Array<[Tab, string, number?, number?]> = [
    ["friends", "Friends"],
    ["followers", "Followers", undefined, followerTotal],
    ["following", "Following", undefined, followingTotal],
    ["blocked", "Blocked"],
  ];

  const list = (items: Friend[], note: (f: Friend) => string | undefined) => (
    <ul className="user-list">{items.map((f) => <li key={f.user.id}><UserRow user={f.user} note={note(f)} actions={<FriendButtons user={f.user} compact />} /></li>)}</ul>
  );

  let body;
  if (status === "idle" || status === "loading") body = <ListSkeleton rows={6} />;
  else if (status === "error") body = <ErrorState message={error} onRetry={() => void loadFriends()} />;
  else if (tab === "friends") {
    // New followers are shown right here (with "Follow back") so nobody has to hunt for the button.
    body = groups.incoming.length + groups.friends.length === 0 ? (
      q ? <EmptyState icon="search" title="No matches" message={`Nothing found for “${q}”.`} /> : <EmptyState icon="users" title="No friends yet" message="Follow people you know. When you follow each other you become friends and can chat." actions={<Button size="sm" onClick={() => navigate("/search")}>Find people</Button>} />
    ) : (
      <>
        {groups.incoming.length > 0 && <><h3 className="list-heading">New followers · {groups.incoming.length}</h3>{list(groups.incoming, () => "follows you")}</>}
        {groups.friends.length > 0 && <>{groups.incoming.length > 0 && <h3 className="list-heading">Friends · {groups.friends.length}</h3>}{list(groups.friends, () => "following each other")}</>}
      </>
    );
  } else if (tab === "followers") {
    const items = [...groups.incoming, ...groups.friends];
    body = items.length ? list(items, (f) => (f.status === "friends" ? "following each other" : "follows you")) : <EmptyState icon="user-plus" title="No followers yet" message="People who follow you show up here." />;
  } else if (tab === "following") {
    const items = [...groups.outgoing, ...groups.friends];
    body = items.length ? list(items, (f) => (f.status === "friends" ? "following each other" : "waiting for a follow back")) : <EmptyState icon="user-plus" title="Not following anyone" message="Search for people and tap Follow." actions={<Button size="sm" onClick={() => navigate("/search")}>Find people</Button>} />;
  } else {
    body = groups.blocked.length ? list(groups.blocked, () => "blocked") : <EmptyState icon="block" title="No blocked users" message="People you block can't message you." />;
  }

  return (
    <div className="page">
      <div className="page-inner">
        <h2 className="page-title">Friends</h2>
        <div className="tabs" role="tablist" aria-label="Friends sections">
          {tabs.map(([k, label, , total]) => (
            <button key={k} type="button" role="tab" className="tab" aria-selected={tab === k} onClick={() => setTab(k)}>
              {label}
              {total ? <span style={{ marginLeft: 6, opacity: 0.7 }}>{total}</span> : null}
              {k === "friends" && newFollowers ? <span className={cn("badge")} style={{ marginLeft: 6 }}>{newFollowers}</span> : null}
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
