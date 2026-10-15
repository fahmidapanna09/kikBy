import type { Settings } from "@/types/models";
import type { UserService } from "@/services/types";
import { ServiceError } from "@/services/errors";
import { http } from "@/services/http";
import { normalizePhone } from "@/lib/validation";
import { session } from "@/services/session";
import { userService as mockUsers } from "@/mock/userService";
import { fromPublic, toUser, type ApiMe, type ApiPublicProfile } from "./mappers";

/**
 * Live profile endpoints. Everything the Go backend does not expose in the shape the UI needs yet
 * (user search/profile by id) keeps using the mock
 * implementation — see README "Going live" for the exact list.
 */
let settingsQueue: Promise<unknown> = Promise.resolve();

export const liveUserService: UserService = {
  ...mockUsers,
  /** Settings sync across devices. The mock store doubles as the local cache (and supplies defaults). */
  async getSettings() {
    try {
      const r = await http<{ settings: Settings | null }>("/users/me/settings");
      if (r.settings) return mockUsers.updateSettings(r.settings); // refresh the local copy; returns defaults-merged settings
    } catch {
      /* offline / old server: fall back to the local copy */
    }
    return mockUsers.getSettings();
  },
  async updateSettings(patch) {
    const next = await mockUsers.updateSettings(patch); // persists locally first so a reload never loses a toggle
    // PUTs are serialised and always send the LATEST local copy, so two quick toggles can never land out of order
    // and let the older one overwrite the newer one on the server.
    const job = settingsQueue.then(async () => http("/users/me/settings", { method: "PUT", body: await mockUsers.getSettings() }));
    settingsQueue = job.catch(() => undefined);
    await job;
    return next;
  },
  async me() {
    const u = toUser(await http<ApiMe>("/users/me"));
    session.patchUser(u);
    return u;
  },
  async getProfile(id) {
    // The mock db's copy of "me" is only a seed; the server is the source of truth for the signed-in user.
    if (id === session.get()?.user.id) return liveUserService.me();
    return fromPublic(await http<ApiPublicProfile>(`/users/${encodeURIComponent(id)}`));
  },
  async search(query) {
    const q = query.trim().replace(/^@/, "").toLowerCase();
    if (q.length < 2) return [];
    // backend search is a username PREFIX match (rate-limited server-side); display-name search is not supported live
    const r = await http<{ users: ApiPublicProfile[] }>(`/users/search?username=${encodeURIComponent(q)}`);
    return (r.users ?? []).map(fromPublic);
  },
  async changeEmail({ email, password }) {
    // Wrong password → 422 with fields.password (never 401, so a typo cannot sign anyone out).
    await http("/auth/change-email", { method: "POST", body: { password: password ?? "", new_email: email.trim() } });
    const user = toUser(await http<ApiMe>("/users/me")); // now carries pending_email
    session.patchUser(user);
    return { user, pending: true };
  },
  async changePassword({ current, next }) {
    // Signs every OTHER device out; this session stays valid.
    await http("/auth/change-password", { method: "POST", body: { current_password: current, new_password: next } });
  },
  async deleteAccount({ password }) {
    await http("/auth/delete-account", { method: "POST", body: { password } });
    session.clear(); // the server revoked every session; nothing left to refresh
  },
  async updateProfile(patch) {
    if (patch.email !== undefined && patch.email !== session.get()?.user.email) {
      throw new ServiceError("not_supported", "Use the e-mail form: it needs your password.", { fields: { email: "Use the e-mail form: it needs your password." } });
    }
    if (patch.username !== undefined && patch.username !== session.get()?.user.username) {
      await http("/users/username", { method: "PATCH", body: { username: patch.username } }).catch((e) => {
        if (e instanceof ServiceError && /taken/i.test(e.message)) e.fields = { username: "That username is taken" };
        throw e;
      });
    }
    const body: Record<string, unknown> = {};
    if (patch.displayName !== undefined) body.display_name = patch.displayName;
    if (patch.bio !== undefined) body.about = patch.bio;
    if (patch.phone !== undefined) body.contact_phone = normalizePhone(patch.phone); // backend expects E.164; "" removes it
    const me = Object.keys(body).length ? await http<ApiMe>("/users/me", { method: "PATCH", body }) : await http<ApiMe>("/users/me");
    const u = toUser(me);
    session.patchUser(u);
    return u;
  },
};
