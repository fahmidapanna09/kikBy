import { useEffect } from "react";
import type { ReactNode } from "react";
import { Redirect, findRoute, useLocation } from "@/lib/router";
import { session, useSession } from "@/services/session";
import { config } from "@/services/config";
import DesignSystemPage from "@/pages/DesignSystemPage";
import LoginPage from "@/pages/auth/LoginPage";
import RegisterPage from "@/pages/auth/RegisterPage";
import ForgotPasswordPage from "@/pages/auth/ForgotPasswordPage";
import ResetPasswordPage from "@/pages/auth/ResetPasswordPage";
import VerifyEmailPage from "@/pages/auth/VerifyEmailPage";
import HomePage, { HomePanel } from "@/pages/HomePage";
import ChatsPage from "@/pages/chats/ChatsPage";
import FriendsPage from "@/pages/FriendsPage";
import ProfilePage from "@/pages/ProfilePage";
import NotificationsPage from "@/pages/NotificationsPage";
import SearchPage from "@/pages/SearchPage";
import SettingsPage from "@/pages/settings/SettingsPage";
import { AppShell } from "@/components/layout/AppShell";

interface RouteDef {
  path: string;
  guard?: "auth" | "guest" | "dev";
  render: (p: { params: Record<string, string>; query: URLSearchParams }) => ReactNode;
  /** content for the desktop right-hand panel */
  panel?: (p: { params: Record<string, string> }) => ReactNode;
}


const routes: RouteDef[] = [
  { path: "/_ds", guard: "dev", render: () => <DesignSystemPage /> },
  { path: "/login", guard: "guest", render: () => <LoginPage /> },
  { path: "/register", guard: "guest", render: () => <RegisterPage /> },
  { path: "/forgot-password", guard: "guest", render: () => <ForgotPasswordPage /> },
  { path: "/reset-password", render: () => <ResetPasswordPage /> },
  { path: "/verify-email", guard: "auth", render: () => <VerifyEmailPage /> },
  { path: "/", guard: "auth", render: () => <HomePage />, panel: () => <HomePanel /> },
  { path: "/chats", guard: "auth", render: () => <ChatsPage /> },
  { path: "/chats/:id", guard: "auth", render: ({ params }) => <ChatsPage id={params.id} /> },
  { path: "/friends", guard: "auth", render: () => <FriendsPage /> },
  { path: "/profile", guard: "auth", render: () => <ProfilePage /> },
  { path: "/profile/:id", guard: "auth", render: ({ params }) => <ProfilePage id={params.id} key={params.id} /> },
  { path: "/notifications", guard: "auth", render: () => <NotificationsPage /> },
  { path: "/search", guard: "auth", render: () => <SearchPage /> },
  { path: "/settings", guard: "auth", render: () => <SettingsPage /> },
  { path: "/settings/:tab", guard: "auth", render: ({ params }) => <SettingsPage tab={params.tab} /> },
];

export function App() {
  const { path, query } = useLocation();
  const sess = useSession();
  const authed = !!sess && sess.expiresAt > Date.now();

  // Expire sessions while the tab stays open.
  useEffect(() => {
    if (!sess) return;
    const ms = Math.max(1000, Math.min(sess.expiresAt - Date.now(), 2 ** 31 - 1));
    const t = setTimeout(() => session.clear(), ms);
    return () => clearTimeout(t);
  }, [sess]);

  const hit = findRoute(routes, path);
  if (!hit) return <Redirect to={authed ? "/" : "/login"} />;
  const { route, match } = hit;
  if (route.guard === "dev" && config.liveAuth) return <Redirect to={authed ? "/" : "/login"} />; // design-system page is mock/dev only
  if (route.guard === "auth" && !authed) return <Redirect to={`/login?next=${encodeURIComponent(path)}`} />;
  if (route.guard === "guest" && authed) return <Redirect to="/" />;
  const content = route.render({ params: match.params, query });
  if (route.guard !== "auth") return <>{content}</>;
  return <AppShell panel={route.panel?.({ params: match.params })}>{content}</AppShell>;
}
