import { useEffect, useState } from "react";
import { IconButton } from "@/components/common/Button";
import { Icon } from "@/icons/Icon";
import { LogoMark } from "@/icons/Logo";
import { navigate, useLocation } from "@/lib/router";
import { useBadges } from "@/hooks/useBadges";

const TITLES: Array<[string, string]> = [["/chats", "Chats"], ["/friends", "Friends"], ["/notifications", "Notifications"], ["/search", "Search"], ["/settings", "Settings"], ["/profile", "Profile"]];

export function TopBar() {
  const { path, query } = useLocation();
  const [q, setQ] = useState(query.get("q") ?? "");
  const { notifications } = useBadges();
  const urlQ = query.get("q") ?? "";
  useEffect(() => { if (path === "/search") setQ(urlQ); }, [path, urlQ]);
  const title = path === "/" ? "Home" : TITLES.find(([p]) => path.startsWith(p))?.[1] ?? "KikBite";
  return (
    <header className="topbar">
      <span className="topbar-brand"><LogoMark size={28} /></span>
      <h1 className="topbar-title">{title}</h1>
      <form className="topbar-search" role="search" onSubmit={(e: any) => { e.preventDefault(); navigate(`/search${q.trim() ? `?q=${encodeURIComponent(q.trim())}` : ""}`); }}>
        <Icon name="search" size={18} />
        <input type="search" value={q} onChange={(e: any) => setQ(e.target.value)} placeholder="Search people and messages" aria-label="Search" maxLength={100} />
      </form>
      <span className="topbar-spacer" />
      <span className="topbar-search-btn"><IconButton label="Search" onClick={() => navigate("/search")}><Icon name="search" /></IconButton></span>
      <IconButton label={notifications ? `Notifications, ${notifications} unread` : "Notifications"} onClick={() => navigate("/notifications")} badge={notifications > 0}>
        <Icon name="bell" />
      </IconButton>
    </header>
  );
}
