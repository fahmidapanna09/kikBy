import { Icon } from "@/icons/Icon";
import { Link, useLocation } from "@/lib/router";
import { useBadges } from "@/hooks/useBadges";
import { NAV_ITEMS, isActive } from "./nav";

/** Bottom tab bar (mobile only). Mirrors the design's bottom nav: icon + label, gradient underline on the active tab. */
export function MobileNav() {
  const { path } = useLocation();
  const badges = useBadges();
  const items = [...NAV_ITEMS.filter((i) => i.to !== "/search"), { to: "/profile", label: "Me", icon: "user" as const, match: ["/profile", "/settings"] }];
  return (
    <nav className="mobile-nav" aria-label="Primary">
      {items.map((it) => {
        const active = isActive(it, path);
        const n = "badge" in it && it.badge ? badges[it.badge] : 0;
        return (
          <Link key={it.to} to={it.to} className="mnav-item" aria-current={active ? "page" : undefined} aria-label={n ? `${it.label}, ${n} new` : it.label}>
            <span className="mnav-icon">
              <Icon name={it.icon} />
              {n > 0 && <span className="badge side-badge" aria-hidden="true">{n > 99 ? "99+" : n}</span>}
            </span>
            <span className="mnav-label">{it.label}</span>
          </Link>
        );
      })}
    </nav>
  );
}
