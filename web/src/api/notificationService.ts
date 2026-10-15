import type { NotificationService } from "@/services/types";
import { http } from "@/services/http";
import { toNotification, type ApiNotification } from "./mappers";

export const liveNotificationService: NotificationService = {
  async list() {
    const r = await http<{ notifications: ApiNotification[] }>("/notifications");
    return (r.notifications ?? []).map(toNotification);
  },
  async markRead(id) {
    await http(`/notifications/${encodeURIComponent(id)}/read`, { method: "POST" });
  },
  async markAllRead() {
    await http("/notifications/read-all", { method: "POST" });
  },
};
