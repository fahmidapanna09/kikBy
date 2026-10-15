import { Avatar } from "@/components/common/Avatar";
import { Menu } from "@/components/common/Dropdown";
import { Icon } from "@/icons/Icon";
import { LogoMark } from "@/icons/Logo";
import { Link, navigate, useLocation } from "@/lib/router";
import { useBadges } from "@/hooks/useBadges";
import { services, useSession } from "@/services";
import { useState } from "react";
import { NAV_ITEMS, isActive } from "./nav";

export function Sidebar() {
  const { path } = useLocation();
  const badges = useBadges();
  const me = useSession()?.user;
  const [menuAt, setMenuAt] = useState<{ x: number; y: number } | null>(null);
  return (
    <nav className="sidebar" aria-label="Primary">
      <Link to="/" className="sidebar-logo" aria-label="KikBite home">
        <LogoMark size={34} />
      </Link>
      <ul className="sidebar-nav">
        {NAV_ITEMS.map((it) => {
          const active = isActive(it, path);
          const n = it.badge ? badges[it.badge] : 0;
          return (
            <li key={it.to}>
              <Link to={it.to} className="side-item" aria-current={active ? "page" : undefined} aria-label={n ? `${it.label}, ${n} new` : it.label} title={it.label}>
                <Icon name={it.icon} />
                {n > 0 && <span className="badge side-badge" aria-hidden="true">{n > 99 ? "99+" : n}</span>}
              </Link>
            </li>
          );
        })}
      </ul>
      <div className="sidebar-foot">
        <Link to="/settings" className="side-item" aria-current={path.startsWith("/settings") ? "page" : undefined} aria-label="Settings" title="Settings">
          <Icon name="settings" />
        </Link>
        {me && (
          <button type="button" className="side-avatar" aria-label="Account menu" aria-haspopup="menu" aria-expanded={!!menuAt} onClick={(e: any) => { const r = e.currentTarget.getBoundingClientRect(); setMenuAt({ x: r.right + 8, y: r.top - 120 }); }}>
            <Avatar name={me.displayName} id={me.id} src={me.avatarUrl} size={36} presence="online" />
          </button>
        )}
      </div>
      {menuAt && (
        <Menu
          x={menuAt.x}
          y={menuAt.y}
          onClose={() => setMenuAt(null)}
          items={[
            { label: "My profile", icon: "user", onSelect: () => navigate("/profile") },
            { label: "Settings", icon: "settings", onSelect: () => navigate("/settings") },
            { label: "Sign out", icon: "logout", danger: true, separatorBefore: true, onSelect: () => void services.auth.logout() },
          ]}
        />
      )}
    </nav>
  );
}
