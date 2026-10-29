import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { EmptyState, ErrorState } from "@/components/common/EmptyState";
import { Skeleton } from "@/components/common/Skeleton";
import { ConversationItem } from "@/components/chat/ConversationItem";
import { Icon } from "@/icons/Icon";
import type { IconName } from "@/icons/Icon";
import { Link, navigate } from "@/lib/router";
import { useBadges } from "@/hooks/useBadges";
import { loadConversations } from "@/state/actions";
import { useApp } from "@/state/store";
import { useSession } from "@/services";

function greeting() {
  const h = new Date().getHours();
  return h < 5 ? "Still up" : h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
}

function Stat({ icon, label, value, to }: { icon: IconName; label: string; value: number; to: string }) {
  return (
    <Link to={to} className="stat-card" aria-label={`${label}: ${value}`}>
      <span className="stat-icon"><Icon name={icon} /></span>
      <span className="stat-value">{value}</span>
      <span className="stat-label">{label}</span>
    </Link>
  );
}

export default function HomePage() {
  const me = useSession()?.user;
  const badges = useBadges();
  const status = useApp((s) => s.convStatus);
  const convs = useApp((s) => s.conversations);
  const error = useApp((s) => s.convError);
  const recent = [...convs].filter((c) => !c.archived).sort((a, b) => +new Date(b.updatedAt) - +new Date(a.updatedAt)).slice(0, 5);
  return (
    <div className="page">
      <div className="page-inner">
        <header className="home-hero">
          <h2>{greeting()}, <span className="grad-text">{me?.displayName.split(" ")[0]}</span></h2>
          <p>Here's what's happening in your conversations.</p>
        </header>
        <div className="stat-grid">
          <Stat icon="chat" label="Unread messages" value={badges.chats} to="/chats" />
          <Stat icon="user-plus" label="New followers" value={badges.friends} to="/friends" />
          <Stat icon="bell" label="Notifications" value={badges.notifications} to="/notifications" />
        </div>
        <div className="quick-actions">
          <Button onClick={() => navigate("/chats")}><Icon name="chat" size={18} /> Open chats</Button>
          <Button variant="secondary" onClick={() => navigate("/search")}><Icon name="search" size={18} /> Find people</Button>
        </div>
        <section aria-labelledby="recent-h" className="card">
          <div className="card-head">
            <h3 id="recent-h">Recent conversations</h3>
            <Link to="/chats">See all</Link>
          </div>
          {status === "loading" || status === "idle" ? (
            <div role="status" aria-label="Loading conversations" style={{ padding: 16, display: "grid", gap: 16 }}>
              {[0, 1, 2].map((i) => <div key={i} style={{ display: "flex", gap: 12 }}><Skeleton w={48} h={48} circle /><div style={{ flex: 1, display: "grid", gap: 8 }}><Skeleton w="45%" /><Skeleton w="70%" h={11} /></div></div>)}
            </div>
          ) : status === "error" ? (
            <ErrorState message={error} onRetry={() => void loadConversations()} />
          ) : recent.length === 0 ? (
            <EmptyState icon="chat" title="No conversations yet" message="Find a friend and say hi." actions={<Button size="sm" onClick={() => navigate("/search")}>Find people</Button>} />
          ) : (
            <ul>{recent.map((c) => <li key={c.id}><ConversationItem conv={c} meId={me?.id ?? ""} compact /></li>)}</ul>
          )}
        </section>
      </div>
    </div>
  );
}

/** Right-hand panel on Home: who's online right now. */
export function HomePanel() {
  const friends = useApp((s) => s.friends);
  const users = useApp((s) => s.users);
  const online = friends.filter((f) => f.status === "friends" && users[f.user.id]?.presence === "online");
  return (
    <div className="panel-section">
      <h3 className="panel-title">Online now <span className="badge muted">{online.length}</span></h3>
      {online.length === 0 ? <p className="field-hint">No friends online right now.</p> : (
        <ul className="panel-list">
          {online.map((f) => (
            <li key={f.user.id}>
              <Link to={`/profile/${f.user.id}`} className="panel-row">
                <Avatar name={f.user.displayName} id={f.user.id} src={f.user.avatarUrl} size={36} presence="online" />
                <span className="truncate">{f.user.displayName}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
