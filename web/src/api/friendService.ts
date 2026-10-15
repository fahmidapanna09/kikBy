import type { FriendService } from "@/services/types";
import { http } from "@/services/http";
import { toFriend, type ApiRelation } from "./mappers";

const enc = encodeURIComponent;

export const liveFriendService: FriendService = {
  async list() {
    const r = await http<{ friends: ApiRelation[] }>("/friends");
    return (r.friends ?? []).map(toFriend);
  },
  async sendRequest(userId) {
    return toFriend(await http<ApiRelation>("/friends/requests", { method: "POST", body: { user_id: userId } }));
  },
  async accept(userId) {
    return toFriend(await http<ApiRelation>(`/friends/${enc(userId)}/accept`, { method: "POST" }));
  },
  async reject(userId) {
    // the same endpoint declines (I am the addressee) or cancels (I am the requester)
    await http(`/friends/requests/${enc(userId)}`, { method: "DELETE" });
  },
  async remove(userId) {
    await http(`/friends/${enc(userId)}`, { method: "DELETE" });
  },
  async block(userId) {
    await http(`/friends/${enc(userId)}/block`, { method: "PUT" });
  },
  async unblock(userId) {
    await http(`/friends/${enc(userId)}/block`, { method: "DELETE" });
  },
};
