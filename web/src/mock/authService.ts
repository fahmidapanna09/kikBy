import { normalizePhone } from "@/lib/validation";
import type { AuthService } from "@/services/types";
import type { AuthSession, User } from "@/types/models";
import { ServiceError } from "@/services/errors";
import { session } from "@/services/session";
import { uid } from "@/lib/utils";
import { delay } from "./util";
import { loadAccounts, mockHash, resetDb, saveAccounts } from "./db";

const resetTokens = new Map<string, string>(); // token -> account email
let lastResetLink = "";
/** Mock mode only: lets the UI show the "email" link since no email is really sent. */
export const mockLastResetLink = () => lastResetLink;

function makeSession(user: User, remember: boolean): AuthSession {
  return {
    user,
    accessToken: `mock.${uid("at")}`,
    refreshToken: `mock.${uid("rt")}`,
    expiresAt: Date.now() + (remember ? 30 * 86400000 : 8 * 3600000),
    remember,
  };
}

export const authService: AuthService = {
  async login({ identifier, password, remember }) {
    await delay(400, 800);
    const id = identifier.trim().toLowerCase();
    const acc = loadAccounts().find((a) => a.user.username === id || a.user.email?.toLowerCase() === id);
    // Same message for unknown user / wrong password: no account enumeration.
    if (!acc || acc.pwHash !== mockHash(password)) throw new ServiceError("invalid_credentials", "Incorrect email/username or password.", { status: 401 });
    const s = makeSession({ ...acc.user, presence: "online" }, remember);
    session.set(s);
    return s;
  },

  async register({ name, username, email, password, phone }) {
    await delay(500, 900);
    const accounts = loadAccounts();
    const fields: Record<string, string> = {};
    if (accounts.some((a) => a.user.username === username.trim().toLowerCase())) fields.username = "That username is taken";
    if (accounts.some((a) => a.user.email?.toLowerCase() === email.trim().toLowerCase())) fields.email = "An account with this email already exists";
    if (Object.keys(fields).length) throw new ServiceError("validation", "Please fix the highlighted fields.", { status: 422, fields });
    const user: User = {
      id: uid("u"), username: username.trim().toLowerCase(), displayName: name.trim(), email: email.trim().toLowerCase(), ...(phone && normalizePhone(phone) ? { phone: normalizePhone(phone) } : {}),
      presence: "online", createdAt: new Date().toISOString(),
    };
    saveAccounts([...accounts, { user, pwHash: mockHash(password) }]);
    const s = makeSession(user, false);
    session.set(s);
    return s;
  },

  async forgotPassword(email) {
    await delay(500, 800);
    // Always resolves — never reveal whether the address has an account.
    const acc = loadAccounts().find((a) => a.user.email?.toLowerCase() === email.trim().toLowerCase());
    if (acc) {
      const token = uid("rst");
      resetTokens.set(token, acc.user.email as string);
      lastResetLink = `/reset-password?token=${token}`;
      console.info("[mock] password reset link:", lastResetLink);
    } else lastResetLink = "";
  },

  async resetPassword({ token, password }) {
    await delay(500, 800);
    const email = resetTokens.get(token);
    if (!email) throw new ServiceError("invalid_token", "This reset link is invalid or has expired. Request a new one.", { status: 400 });
    const accounts = loadAccounts();
    saveAccounts(accounts.map((a) => (a.user.email === email ? { ...a, pwHash: mockHash(password) } : a)));
    resetTokens.delete(token);
  },

  async verifyEmail(code) {
    await delay(300, 500);
    if (!/^\d{6}$/.test(code) || code === "000000") throw new ServiceError("invalid_code", "That code isn't right — check it and try again.", { status: 400 });
  },

  async resendVerification() {
    await delay(200, 400);
    return { alreadyVerified: true }; // mock accounts have no verification step
  },

  async logout() {
    await delay(80, 200);
    session.clear();
    resetDb();
  },
};
