import { http } from "@/services/http";
import { navigate } from "@/lib/router";

/**
 * Firebase push for the Android app (Capacitor "PushNotifications" plugin, reached through the bridge Capacitor injects
 * into the page). It is what lets a notification arrive while the app is closed. Best-effort and silent everywhere else:
 * no plugin / no permission / no Firebase config in the build → nothing happens.
 */
interface PushPlugin {
  requestPermissions(): Promise<{ receive: string }>;
  register(): Promise<void>;
  createChannel?(c: { id: string; name: string; description?: string; importance: number; visibility?: number; vibration?: boolean }): Promise<void>;
  addListener(ev: string, cb: (e: any) => void): unknown;
}
interface CapacitorGlobal {
  isNativePlatform?: () => boolean;
  Plugins?: Record<string, unknown>;
  registerPlugin?: (name: string) => unknown;
}

function plugin(): PushPlugin | null {
  const c = (window as unknown as { Capacitor?: CapacitorGlobal }).Capacitor;
  if (!c?.isNativePlatform?.()) return null;
  try {
    return ((c.Plugins?.PushNotifications ?? c.registerPlugin?.("PushNotifications")) as PushPlugin) ?? null;
  } catch {
    return null;
  }
}

let started = false;
let lastToken = "";

/** Ask for permission, get this phone's FCM token and hand it to the server. Call after sign-in. */
export async function setupPush(): Promise<void> {
  const p = plugin();
  if (!p) return;
  if (!started) {
    started = true;
    p.addListener("registration", (t: { value?: string }) => {
      if (t?.value) void sendToken(t.value);
    });
    p.addListener("registrationError", (e: unknown) => console.warn("push: registration failed", e));
    p.addListener("pushNotificationActionPerformed", (a: { notification?: { data?: { path?: string } } }) => {
      const path = a?.notification?.data?.path;
      if (typeof path === "string" && path.startsWith("/")) navigate(path);
    });
  }
  try {
    const perm = await p.requestPermissions();
    if (perm.receive !== "granted") return;
    // Loud channel for incoming calls (the server's call pushes name it); messages use the "kikbite" channel.
    await p.createChannel?.({ id: "kikbite_calls", name: "Incoming calls", description: "Rings when someone calls you", importance: 5, visibility: 1, vibration: true }).catch(() => undefined);
    await p.register();
  } catch (e) {
    console.warn("push: setup failed", e);
  }
}

async function sendToken(token: string): Promise<void> {
  lastToken = token;
  try {
    await http("/push/token", { method: "PUT", body: { token }, auth: true });
  } catch {
    /* retried on the next app start */
  }
}

/** Sign-out: stop this phone from receiving the account's notifications. */
export async function dropPushToken(): Promise<void> {
  if (!lastToken) return;
  try {
    await http("/push/token", { method: "DELETE", auth: true });
  } catch {
    /* best effort */
  }
  lastToken = "";
}
