import { useCallback, useEffect, useSyncExternalStore } from "react";
import type { ReactNode } from "react";

/* Tiny History-API router: route table with :params, query parsing, Link, navigate(). */

export interface RouteMatch {
  pattern: string;
  params: Record<string, string>;
}

let listeners = new Set<() => void>();
const snapshot = () => window.location.pathname + window.location.search;
const subscribe = (cb: () => void) => {
  listeners.add(cb);
  window.addEventListener("popstate", cb);
  return () => {
    listeners.delete(cb);
    window.removeEventListener("popstate", cb);
  };
};

export function navigate(to: string, opts: { replace?: boolean } = {}): void {
  const target = to.startsWith("/") ? to : `/${to}`;
  if (target === snapshot()) return;
  if (opts.replace) window.history.replaceState({}, "", target);
  else window.history.pushState({}, "", target);
  listeners.forEach((l) => l());
}

export function useLocation(): { path: string; query: URLSearchParams } {
  const raw = useSyncExternalStore(subscribe, snapshot, () => "/");
  const [path, search = ""] = raw.split("?");
  return { path: path || "/", query: new URLSearchParams(search) };
}

export function matchPath(pattern: string, path: string): RouteMatch | null {
  const pp = pattern.split("/").filter(Boolean);
  const ap = path.split("/").filter(Boolean);
  const wildcard = pp[pp.length - 1] === "*";
  if (!wildcard && pp.length !== ap.length) return null;
  if (wildcard && ap.length < pp.length - 1) return null;
  const params: Record<string, string> = {};
  for (let i = 0; i < pp.length; i++) {
    if (pp[i] === "*") break;
    if (pp[i].startsWith(":")) {
      try {
        params[pp[i].slice(1)] = decodeURIComponent(ap[i]);
      } catch {
        return null;
      }
    } else if (pp[i] !== ap[i]) return null;
  }
  return { pattern, params };
}

export function findRoute<T extends { path: string }>(routes: T[], path: string): { route: T; match: RouteMatch } | null {
  for (const route of routes) {
    const match = matchPath(route.path, path);
    if (match) return { route, match };
  }
  return null;
}

interface LinkProps {
  to: string;
  className?: string;
  children?: ReactNode;
  replace?: boolean;
  onClick?: (e: any) => void;
  "aria-label"?: string;
  "aria-current"?: "page" | undefined;
  title?: string;
}

/** Real <a href> so middle-click / open-in-new-tab / screen readers all work. */
export function Link({ to, replace, onClick, children, ...rest }: LinkProps) {
  const handle = useCallback(
    (e: any) => {
      onClick?.(e);
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      e.preventDefault();
      navigate(to, { replace });
    },
    [to, replace, onClick],
  );
  return (
    <a href={to} onClick={handle} {...rest}>
      {children}
    </a>
  );
}

export function Redirect({ to }: { to: string }) {
  useEffect(() => navigate(to, { replace: true }), [to]);
  return null;
}
