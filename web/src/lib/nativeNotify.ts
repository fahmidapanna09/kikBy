import { navigate } from "@/lib/router";

/**
 * System notifications for the Android app (Capacitor "LocalNotifications" plugin, reached through the bridge
 * Capacitor injects into the page) with a plain Notification API fallback for browsers.
 *
 * Everything here is best-effort: if permission is missing or the plugin is not present, nothing happens.
 * They are shown only while the app's real-time connection is alive (foreground, or shortly after leaving it).
 */
interface LocalNotificationsPlugin {
  checkPermissions(): Promise<{ display: string }>;
  requestPermissions(): Promise<{ display: string }>;
  createChannel?(c: { id: string; name: string; importance: number; vibration?: boolean; visibility?: number }): Promise<void>;
  schedule(o: { notifications: Array<{ id: number; title: string; body: string; channelId?: string; extra?: unknown; schedule?: { at: Date } }> }): Promise<unknown>;
  addListener(ev: string, cb: (e: { notification?: { extra?: { path?: string } } }) => void): unknown;
}
interface CapacitorGlobal {
  isNativePlatform?: () => boolean;
  Plugins?: Record<string, unknown>;
  registerPlugin?: (name: string) => unknown;
}

function cap(): CapacitorGlobal | undefined {
  return (window as unknown as { Capacitor?: CapacitorGlobal }).Capacitor;
}

function plugin(): LocalNotificationsPlugin | null {
  const c = cap();
  if (!c?.isNativePlatform?.()) return null;
  try {
    return ((c.Plugins?.LocalNotifications ?? c.registerPlugin?.("LocalNotifications")) as LocalNotificationsPlugin) ?? null;
  } catch {
    return null;
  }
}

let ready: Promise<boolean> | null = null;
let seq = Math.floor(Date.now() / 1000) % 1_000_000;

/** Ask for permission (Android 13+ shows the system dialog) and create the channel. Safe to call repeatedly. */
export function setupNotifications(): Promise<boolean> {
  if (ready) return ready;
  ready = (async () => {
    const p = plugin();
    if (p) {
      try {
        let st = await p.checkPermissions();
        if (st.display !== "granted") st = await p.requestPermissions();
        if (st.display !== "granted") return false;
        await p.createChannel?.({ id: "kikbite", name: "Messages & calls", importance: 4, vibration: true, visibility: 0 });
        p.addListener("localNotificationActionPerformed", (e) => {
          const path = e.notification?.extra?.path;
          if (typeof path === "string" && path.startsWith("/")) navigate(path);
        });
        return true;
      } catch {
        return false;
      }
    }
    if (typeof Notification === "undefined") return false;
    if (Notification.permission === "granted") return true;
    if (Notification.permission === "denied") return false;
    try {
      return (await Notification.requestPermission()) === "granted";
    } catch {
      return false;
    }
  })();
  return ready;
}

/** Show a system notification. `path` is where tapping it should go (e.g. /chats/<id>). */
export async function notify(title: string, body: string, path?: string): Promise<void> {
  try {
    if (!(await setupNotifications())) return;
    const p = plugin();
    if (p) {
      seq = (seq + 1) % 2_000_000_000;
      await p.schedule({ notifications: [{ id: seq, title, body: body.slice(0, 300), channelId: "kikbite", extra: { path } }] });
      return;
    }
    const n = new Notification(title, { body: body.slice(0, 300), tag: path });
    n.onclick = () => {
      window.focus();
      if (path) navigate(path);
      n.close();
    };
  } catch {
    /* best-effort */
  }
}
