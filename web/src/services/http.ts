import { config } from "./config";
import { handleUnauthorized, session } from "./session";
import { ServiceError } from "./errors";

type Init = { method?: string; body?: unknown; signal?: AbortSignal; auth?: boolean };

let refreshing: Promise<boolean> | null = null;
/** Registered by api/authService (live mode): exchange the refresh token for a new pair. Single-flight. */
let refresher: (() => Promise<boolean>) | null = null;
export const setRefresher = (fn: () => Promise<boolean>) => {
  refresher = fn;
};
function refreshOnce(): Promise<boolean> {
  if (!refresher) return Promise.resolve(false);
  refreshing ??= refresher().finally(() => {
    refreshing = null;
  });
  return refreshing;
}

async function send(path: string, init: Init): Promise<Response> {
  const s = init.auth === false ? null : session.get();
  try {
    return await fetch(`${config.apiBase}${path}`, {
      method: init.method ?? "GET",
      headers: { "Content-Type": "application/json", ...(s ? { Authorization: `Bearer ${s.accessToken}` } : {}) },
      body: init.body === undefined ? undefined : JSON.stringify(init.body),
      signal: init.signal,
    });
  } catch {
    throw new ServiceError("network", "You appear to be offline. Check your connection and try again.");
  }
}

/**
 * Fetch wrapper for the REAL API.
 * - attaches the bearer token
 * - 401 on an authenticated call → try ONE silent refresh + retry; if that fails, clear the session (router → /login)
 * - 401 on an unauthenticated call (login) is a normal "wrong credentials" error, never a logout
 * - error bodies { error: { code, message, fields } } become ServiceError
 */
export async function http<T>(path: string, init: Init = {}): Promise<T> {
  let res = await send(path, init);
  if (res.status === 401 && init.auth !== false) {
    if (await refreshOnce()) res = await send(path, init);
    if (res.status === 401) {
      handleUnauthorized();
      throw new ServiceError("unauthorized", "Your session has expired. Please sign in again.", { status: 401 });
    }
  }
  const json = await res.json().catch(() => null);
  if (!res.ok) {
    const e = json?.error ?? {};
    throw new ServiceError(e.code ?? "http_error", e.message ?? `Request failed (${res.status})`, { status: res.status, fields: e.fields });
  }
  return (json?.data ?? json) as T;
}
