import type { Services } from "./types";
import { config } from "./config";
import { authService } from "@/mock/authService";
import { userService } from "@/mock/userService";
import { chatService } from "@/mock/chatService";
import { messageService } from "@/mock/messageService";
import { friendService } from "@/mock/friendService";
import { notificationService } from "@/mock/notificationService";
import { uploadService } from "@/mock/uploadService";
import { liveAuthService, startLiveSession } from "@/api/authService";
import { liveUserService } from "@/api/userService";
import { liveFriendService } from "@/api/friendService";
import { liveNotificationService } from "@/api/notificationService";
import { liveChatService, liveMessageService } from "@/api/chatService";
import { liveUploadService } from "@/api/uploadService";

/**
 * Single composition point for data access. UI imports `services` only.
 * To go live: implement the interfaces in ./types against ./http.ts and return them here
 * when `config.mock === false`. No component changes required.
 */
const mockServices: Services = {
  auth: authService,
  users: userService,
  chats: chatService,
  messages: messageService,
  friends: friendService,
  notifications: notificationService,
  uploads: uploadService,
};

/**
 * Hybrid: real auth, profile, friends, notifications and user search; with `liveChat` also real conversations,
 * messages and the socket. Uploads/attachments go live together with chat.
 */
const hybridServices: Services = {
  ...mockServices,
  auth: liveAuthService,
  users: liveUserService,
  friends: liveFriendService,
  notifications: liveNotificationService,
  ...(config.liveChat ? { chats: liveChatService, messages: liveMessageService, uploads: liveUploadService } : {}),
};

export const services: Services = config.mock
  ? config.liveAuth
    ? (startLiveSession(), hybridServices)
    : mockServices
  : (() => {
      throw new Error("Real API services are not implemented yet — set mock:true or implement ./types against ./http.ts");
    })();

export { mockLastResetLink } from "@/mock/authService";
export * from "./errors";
export * from "./session";
