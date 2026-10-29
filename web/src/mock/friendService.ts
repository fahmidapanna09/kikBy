import type { FriendService } from "@/services/types";
import type { Friend } from "@/types/models";
import { ServiceError } from "@/services/errors";
import { delay, readDelay, requireState } from "./util";

function find(userId: string) {
  const s = requireState();
  const f = s.friends.find((x) => x.user.id === userId);
  return { s, f };
}

export const friendService: FriendService = {
  async list() {
    await readDelay();
    return requireState().friends.map((f) => ({ ...f }));
  },
  async sendRequest(userId) {
    await delay(200, 450);
    const { s, f } = find(userId);
    if (f) {
      if (f.status === "blocked") throw new ServiceError("blocked", "Unblock this user first.", { status: 409 });
      return { ...f };
    }
    const user = s.users[userId];
    if (!user) throw new ServiceError("not_found", "User not found", { status: 404 });
    const nf: Friend = { user, status: "outgoing" };
    s.friends.push(nf);
    return { ...nf };
  },
  async accept(userId) {
    await delay(150, 350);
    const { f } = find(userId);
    if (!f || f.status !== "incoming") throw new ServiceError("not_found", "Request not found", { status: 404 });
    f.status = "friends";
    f.since = new Date().toISOString();
    return { ...f };
  },
  async reject(userId) {
    await delay(150, 300);
    const { s } = find(userId);
    s.friends = s.friends.filter((x) => !(x.user.id === userId && (x.status === "incoming" || x.status === "outgoing")));
  },
  async remove(userId) {
    await delay(150, 300);
    const { s } = find(userId);
    const f = s.friends.find((x) => x.user.id === userId && x.status === "friends");
    if (f) { f.status = "incoming"; f.since = undefined; } // unfollow: they still follow me
  },
  async block(userId) {
    await delay(150, 300);
    const { s, f } = find(userId);
    if (f) {
      f.status = "blocked";
      return;
    }
    const user = s.users[userId];
    if (user) s.friends.push({ user, status: "blocked" });
  },
  async unblock(userId) {
    await delay(150, 300);
    const { s } = find(userId);
    s.friends = s.friends.filter((x) => !(x.user.id === userId && x.status === "blocked"));
  },
};
