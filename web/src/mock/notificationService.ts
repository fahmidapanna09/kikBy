import type { NotificationService } from "@/services/types";
import { readDelay, requireState, delay } from "./util";

export const notificationService: NotificationService = {
  async list() {
    await readDelay();
    return [...requireState().notifications].sort((a, b) => +new Date(b.createdAt) - +new Date(a.createdAt)).map((n) => ({ ...n }));
  },
  async markRead(id) {
    await delay(40, 120);
    const n = requireState().notifications.find((x) => x.id === id);
    if (n) n.read = true;
  },
  async markAllRead() {
    await delay(80, 200);
    requireState().notifications.forEach((n) => (n.read = true));
  },
};
