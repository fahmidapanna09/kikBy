import type { UserService } from "@/services/types";
import { ServiceError } from "@/services/errors";
import { session } from "@/services/session";
import { sanitizeText } from "@/lib/utils";
import { USERNAME_RE, normalizePhone, validatePhone } from "@/lib/validation";
import { delay, readDelay, requireState } from "./util";
import { loadAccounts, loadSettings, mockHash, saveAccounts, saveSettings } from "./db";

export const userService: UserService = {
  async me() {
    await readDelay();
    const s = requireState();
    return s.users[s.meId];
  },
  async getProfile(id) {
    await readDelay();
    const u = requireState().users[id];
    if (!u) throw new ServiceError("not_found", "User not found", { status: 404 });
    return u;
  },
  async updateProfile(patch) {
    await delay(300, 600);
    const s = requireState();
    const me = s.users[s.meId];
    const fields: Record<string, string> = {};
    if (patch.username !== undefined) {
      const un = patch.username.trim().toLowerCase();
      if (!USERNAME_RE.test(un)) fields.username = "3–24 characters: letters, numbers or _";
      else if (loadAccounts().some((a) => a.user.id !== me.id && a.user.username === un)) fields.username = "That username is taken";
      patch = { ...patch, username: un };
    }
    if (patch.email !== undefined) {
      const em = patch.email.trim().toLowerCase();
      if (loadAccounts().some((a) => a.user.id !== me.id && a.user.email?.toLowerCase() === em)) fields.email = "An account with this email already exists";
      patch = { ...patch, email: em };
    }
    if (patch.phone !== undefined) {
      const pe = validatePhone(patch.phone);
      if (pe) fields.phone = pe;
      patch = { ...patch, phone: normalizePhone(patch.phone) || undefined };
    }
    if (Object.keys(fields).length) throw new ServiceError("validation", "Please fix the highlighted fields.", { status: 422, fields });
    const clean = {
      ...patch,
      ...(patch.displayName !== undefined ? { displayName: sanitizeText(patch.displayName, 50).trim() } : {}),
      ...(patch.bio !== undefined ? { bio: sanitizeText(patch.bio, 160) } : {}),
    };
    const next = { ...me, ...clean };
    s.users[s.meId] = next;
    saveAccounts(loadAccounts().map((a) => (a.user.id === me.id ? { ...a, user: next } : a)));
    session.patchUser(clean);
    return next;
  },
  async search(query) {
    await readDelay();
    const s = requireState();
    const q = query.trim().toLowerCase().replace(/^@/, "");
    if (q.length < 2) return [];
    return Object.values(s.users).filter((u) => u.id !== s.meId && (u.username.includes(q) || u.displayName.toLowerCase().includes(q))).slice(0, 20);
  },
  async getSettings() {
    await readDelay();
    return loadSettings(requireState().meId);
  },
  async updateSettings(patch) {
    const s = requireState();
    const next = { ...loadSettings(s.meId), ...patch };
    saveSettings(s.meId, next); // persist first: a reload right after a toggle must not lose it
    await delay(100, 250);
    return next;
  },
  async changeEmail({ email }) {
    return { user: await userService.updateProfile({ email }), pending: false };
  },
  async deleteAccount({ password }) {
    await delay(600, 900);
    const s = requireState();
    const accounts = loadAccounts();
    const acc = accounts.find((a) => a.user.id === s.meId);
    if (!acc || acc.pwHash !== mockHash(password)) throw new ServiceError("validation", "Password is incorrect.", { status: 422, fields: { password: "Password is incorrect" } });
    saveAccounts(accounts.filter((a) => a.user.id !== s.meId));
  },
  async changePassword({ current, next }) {
    await delay(500, 800);
    const s = requireState();
    const accounts = loadAccounts();
    const acc = accounts.find((a) => a.user.id === s.meId);
    if (!acc || acc.pwHash !== mockHash(current)) throw new ServiceError("validation", "Current password is incorrect.", { status: 422, fields: { current: "Current password is incorrect" } });
    saveAccounts(accounts.map((a) => (a.user.id === s.meId ? { ...a, pwHash: mockHash(next) } : a)));
  },
};
