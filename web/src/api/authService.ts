import type { AuthService } from "@/services/types";
import type { AuthSession } from "@/types/models";
import { http, setRefresher } from "@/services/http";
import { session } from "@/services/session";
import { normalizePhone } from "@/lib/validation";
import { deviceId } from "./device";
import { toUser, type ApiMe, type ApiTokens } from "./mappers";

interface ApiAuthResponse {
  user: { id: string; email?: string; username?: string };
  tokens: ApiTokens;
}

const common = () => ({ device_id: deviceId(), platform: "web" });

function applyTokens(prev: AuthSession | null, t: ApiTokens, remember: boolean, user = prev?.user): AuthSession {
  const s: AuthSession = {
    user: user as AuthSession["user"],
    accessToken: t.access_token,
    refreshToken: t.refresh_token,
    expiresAt: Date.parse(t.access_expires_at),
    remember,
  };
  session.set(s);
  return s;
}

/** After tokens are stored: pull the full profile (display name etc.) with the new access token. */
async function completeSession(auth: ApiAuthResponse, remember: boolean): Promise<AuthSession> {
  // provisional user so the bearer token is available to http(); replaced right below
  applyTokens(null, auth.tokens, remember, { id: auth.user.id, username: auth.user.username ?? "", displayName: auth.user.username ?? "", presence: "online", createdAt: new Date().toISOString() });
  try {
    const me = toUser(await http<ApiMe>("/users/me"));
    session.patchUser(me);
  } catch (e) {
    session.clear(); // never leave a half-initialised session behind
    throw e;
  }
  return session.get() as AuthSession;
}

let timer: ReturnType<typeof setTimeout> | undefined;

/** Exchange the refresh token. Rotation: the OLD token is dead after this, so store the new one atomically. */
async function refresh(): Promise<boolean> {
  const s = session.get();
  if (!s?.refreshToken) return false;
  try {
    const t = await http<ApiTokens>("/auth/refresh", { method: "POST", body: { refresh_token: s.refreshToken }, auth: false });
    applyTokens(s, t, s.remember);
    scheduleRefresh();
    return true;
  } catch {
    return false;
  }
}

/**
 * Refresh shortly before the access token expires: 60 s early, or 20 % of the remaining lifetime for
 * short-lived tokens, never sooner than 1 s from now (guards against a refresh storm if the server's TTL is tiny).
 */
export function scheduleRefresh() {
  clearTimeout(timer);
  const s = session.get();
  if (!s) return;
  const remaining = s.expiresAt - Date.now();
  const wait = Math.max(1000, remaining - Math.min(60_000, remaining * 0.2));
  timer = setTimeout(async () => {
    if (!(await refresh()) && session.get()) session.clear();
  }, Math.min(wait, 2 ** 31 - 1));
}

export function startLiveSession() {
  setRefresher(refresh);
  session.subscribe(() => (session.get() ? undefined : clearTimeout(timer)));
  scheduleRefresh();
}

export const liveAuthService: AuthService = {
  async login({ identifier, password, remember }) {
    const r = await http<ApiAuthResponse>("/auth/login", { method: "POST", auth: false, body: { identifier, password, ...common() } });
    const s = await completeSession(r, remember);
    scheduleRefresh();
    return s;
  },
  async register({ name, username, email, password, phone }) {
    const r = await http<ApiAuthResponse>("/auth/register", { method: "POST", auth: false, body: { name, username, email, password, phone: normalizePhone(phone ?? "") || undefined, ...common() } });
    const s = await completeSession(r, true);
    scheduleRefresh();
    return s;
  },
  async forgotPassword(email) {
    await http("/auth/forgot-password", { method: "POST", auth: false, body: { email } });
  },
  async resetPassword({ email, code, password }) {
    await http("/auth/reset-password", { method: "POST", auth: false, body: { email, code, password } });
  },
  async verifyEmail(code) {
    await http("/auth/verify-email", { method: "POST", auth: true, body: { code } });
    // Re-read the profile: clears the banner and shows a newly confirmed address.
    try {
      session.patchUser(toUser(await http<ApiMe>("/users/me")));
    } catch {
      session.patchUser({ emailVerified: true });
    }
  },
  async resendVerification() {
    const r = await http<{ status: string }>("/auth/verify-email/resend", { method: "POST" });
    return { alreadyVerified: r.status === "already_verified" };
  },
  async logout() {
    const s = session.get();
    clearTimeout(timer);
    try {
      if (s) await http("/auth/logout", { method: "POST", body: { refresh_token: s.refreshToken }, auth: true });
    } catch {
      /* best effort: the local session is cleared regardless */
    }
    session.clear();
  },
};
