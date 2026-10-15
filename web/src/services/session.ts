import { useSyncExternalStore } from "react";
import type { AuthSession } from "@/types/models";

/**
 * Session store. "Remember me" → localStorage, otherwise sessionStorage.
 * NOTE (production): tokens in web storage are readable by any XSS. The real backend
 * should prefer an httpOnly+Secure+SameSite refresh cookie; this store then only holds the
 * short-lived access token in memory. The abstraction here lets that swap happen in one file.
 */
const KEY = "kikbite.session";
let current: AuthSession | null = load();
const subs = new Set<() => void>();

function load(): AuthSession | null {
  for (const store of [localStorage, sessionStorage]) {
    try {
      const raw = store.getItem(KEY);
      if (raw) {
        const s = JSON.parse(raw) as AuthSession;
        if (s && s.accessToken && s.user) return s;
      }
    } catch {
      /* corrupted or blocked storage → treat as signed out */
    }
  }
  return null;
}

function persist(s: AuthSession | null) {
  try {
    localStorage.removeItem(KEY);
    sessionStorage.removeItem(KEY);
    if (s) (s.remember ? localStorage : sessionStorage).setItem(KEY, JSON.stringify(s));
  } catch {
    /* storage unavailable — session lives in memory only */
  }
}

export const session = {
  get: () => current,
  set(s: AuthSession) {
    current = s;
    persist(s);
    subs.forEach((f) => f());
  },
  patchUser(patch: Partial<AuthSession["user"]>) {
    if (!current) return;
    session.set({ ...current, user: { ...current.user, ...patch } });
  },
  clear() {
    current = null;
    persist(null);
    subs.forEach((f) => f());
  },
  subscribe(cb: () => void) {
    subs.add(cb);
    return () => subs.delete(cb);
  },
  isValid: () => !!current && current.expiresAt > Date.now(),
};

export const useSession = (): AuthSession | null => useSyncExternalStore(session.subscribe, session.get);

/** Any service that receives HTTP 401 calls this: clear session → router redirects to /login. */
let onUnauthorized: () => void = () => session.clear();
export const setUnauthorizedHandler = (fn: () => void) => {
  onUnauthorized = fn;
};
export const handleUnauthorized = () => onUnauthorized();
