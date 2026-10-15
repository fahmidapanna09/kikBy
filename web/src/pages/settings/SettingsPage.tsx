import type { FC } from "react";
import { Icon } from "@/icons/Icon";
import type { IconName } from "@/icons/Icon";
import { Link, Redirect } from "@/lib/router";
import { useIsMobile } from "@/hooks/useMedia";
import { cn } from "@/lib/utils";
import { AccountSection, AppearanceSection, ChatSection, NotificationsSection, OtherSection, PrivacySection, SecuritySection } from "./sections";

const TABS: Array<{ id: string; label: string; icon: IconName; view: FC }> = [
  { id: "account", label: "Account", icon: "user", view: AccountSection },
  { id: "privacy", label: "Privacy", icon: "lock", view: PrivacySection },
  { id: "notifications", label: "Notifications", icon: "bell", view: NotificationsSection },
  { id: "appearance", label: "Appearance", icon: "palette", view: AppearanceSection },
  { id: "chat", label: "Chat", icon: "chat", view: ChatSection },
  { id: "security", label: "Security", icon: "shield", view: SecuritySection },
  { id: "other", label: "Other", icon: "info", view: OtherSection },
];

/** /settings (menu on mobile, Account on desktop) and /settings/:tab. */
export default function SettingsPage({ tab }: { tab?: string }) {
  const mobile = useIsMobile();
  const active = TABS.find((t) => t.id === tab);
  if (tab && !active) return <Redirect to="/settings" />;
  if (!tab && !mobile) return <Redirect to="/settings/account" />;
  const View = active?.view;
  return (
    <div className="page">
      <div className="page-inner settings-layout">
        {(!mobile || !active) && (
          <nav className="settings-nav" aria-label="Settings sections">
            {mobile && <h2 className="page-title" style={{ marginBottom: 8 }}>Settings</h2>}
            <ul>
              {TABS.map((t) => (
                <li key={t.id}>
                  <Link to={`/settings/${t.id}`} className={cn("settings-link", active?.id === t.id && "on")} aria-current={active?.id === t.id ? "page" : undefined}>
                    <Icon name={t.icon} size={18} /><span>{t.label}</span>{mobile && <Icon name="chevron" size={16} className="settings-chev" />}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>
        )}
        {View && active && (
          <div className="settings-content">
            {mobile && <Link to="/settings" className="btn btn-ghost btn-sm back-link"><Icon name="back" size={16} /> Settings</Link>}
            <h2 className="page-title">{active.label}</h2>
            <View />
          </div>
        )}
      </div>
    </div>
  );
}
