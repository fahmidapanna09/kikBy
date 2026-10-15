import { useEffect } from "react";
import type { ReactNode } from "react";
import { CallOverlay } from "@/components/call/CallOverlay";
import { Icon } from "@/icons/Icon";
import { useNetworkWatcher, useOnline } from "@/hooks/useOnline";
import { useLocation } from "@/lib/router";
import { bootstrap, resetApp } from "@/state/actions";
import { useApp } from "@/state/store";
import { useSession } from "@/services";
import { startRealtime, stopRealtime } from "@/realtime";
import { MobileNav } from "./MobileNav";
import { Sidebar } from "./Sidebar";
import { TopBar } from "./TopBar";
import { VerifyEmailBanner } from "./VerifyEmailBanner";
import { applyAppearance } from "@/lib/appearance";
import { useViewportHeight } from "@/hooks/useViewportHeight";

/**
 * Responsive frame: Sidebar | [page] | RightPanel on ≥768; TopBar + bottom nav on mobile.
 * `immersive` pages (a single open chat) hide the top bar and bottom nav on mobile so the
 * conversation gets the full screen, like the design.
 */
export function AppShell({ children, panel }: { children: ReactNode; panel?: ReactNode }) {
  const me = useSession()?.user;
  const online = useOnline();
  const settings = useApp((s) => s.settings);
  const { path } = useLocation();
  useNetworkWatcher();
  useViewportHeight();

  useEffect(() => {
    if (me) {
      void bootstrap(me.id);
      startRealtime();
    }
    return () => {
      stopRealtime();
      resetApp();
    };
  }, [me?.id]);

  useEffect(() => applyAppearance(settings?.appearance), [settings?.appearance]);

  const immersive = /^\/chats\/[^/]+/.test(path);
  return (
    <div className="shell" data-immersive={immersive}>
      <Sidebar />
      <div className="shell-main">
        {!online && (
          <div className="banner warning" role="status">
            <Icon name="wifi-off" size={16} /> You're offline — messages will send when you reconnect.
          </div>
        )}
        <VerifyEmailBanner />
        <TopBar />
        <div className="shell-body">
          <main id="main" className="shell-content" tabIndex={-1}>{children}</main>
          {panel && <aside className="right-panel" aria-label="Details">{panel}</aside>}
        </div>
        <MobileNav />
      </div>
      <CallOverlay />
    </div>
  );
}
