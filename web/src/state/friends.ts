import { toast } from "@/components/common/Toast";
import { navigate } from "@/lib/router";
import { services } from "@/services";
import { errorMessage } from "@/services/errors";
import type { Friend, FriendStatus, User } from "@/types/models";
import { cacheUsers, removeFriendLocal, setFriend, upsertConversation } from "./actions";
import { store } from "./store";

/** One place for every follow/unfollow/block/message-user operation. Optimistic, with rollback + toast on failure. */
export const statusOf = (userId: string): FriendStatus | "none" => store.get().friends.find((f) => f.user.id === userId)?.status ?? "none";

async function run<T>(user: User, optimistic: () => void, call: () => Promise<T>, ok: string, after?: (r: T) => void) {
  const snapshot = store.get().friends;
  optimistic();
  try {
    const r = await call();
    after?.(r);
    toast.success(ok);
  } catch (e) {
    store.set({ friends: snapshot });
    toast.error(errorMessage(e, `Something went wrong with ${user.displayName}.`));
  }
}

export const friendActions = {
  add: (u: User) =>
    run(u, () => setFriend({ user: u, status: "outgoing" }), () => services.friends.sendRequest(u.id), `You are following ${u.displayName}`),
  accept: (u: User) =>
    run(u, () => setFriend({ user: u, status: "friends", since: new Date().toISOString() }), () => services.friends.accept(u.id), `You followed ${u.displayName} back — you are now friends`, (f: Friend) => setFriend({ ...f, user: u })),
  reject: (u: User) => run(u, () => removeFriendLocal(u.id), () => services.friends.reject(u.id), "Ignored"),
  cancel: (u: User) => run(u, () => removeFriendLocal(u.id), () => services.friends.reject(u.id), `Unfollowed ${u.displayName}`),
  // Unfollowing a friend keeps their follow of me alive: they stay in my list as "incoming" (I can follow back).
  remove: (u: User) => run(u, () => setFriend({ user: u, status: "incoming" }), () => services.friends.remove(u.id), `Unfollowed ${u.displayName}`),
  block: (u: User) => run(u, () => setFriend({ user: u, status: "blocked" }), () => services.friends.block(u.id), `${u.displayName} blocked`),
  unblock: (u: User) => run(u, () => removeFriendLocal(u.id), () => services.friends.unblock(u.id), `${u.displayName} unblocked`),
  async message(u: User) {
    try {
      cacheUsers([u]);
      const c = await services.chats.startDirect(u.id);
      upsertConversation(c);
      navigate(`/chats/${c.id}`);
    } catch (e) {
      toast.error(errorMessage(e, "Couldn't open the chat."));
    }
  },
};
