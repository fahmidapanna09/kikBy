/** Stable per-browser device id sent to the backend (user_devices.device_id). Not a secret. */
const KEY = "kikbite.deviceId";
export function deviceId(): string {
  try {
    let id = localStorage.getItem(KEY);
    if (!id) {
      id = crypto.randomUUID();
      localStorage.setItem(KEY, id);
    }
    return id;
  } catch {
    return (memo ??= crypto.randomUUID());
  }
}
let memo: string | undefined;
