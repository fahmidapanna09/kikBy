import type { ServerEvent } from "@/realtime/events";

/** In-process stand-in for "the server pushing events". Mock services emit here; MockSocket relays. */
type Listener = (e: ServerEvent) => void;
const listeners = new Set<Listener>();
export const serverBus = {
  emit: (e: ServerEvent) => listeners.forEach((l) => l(e)),
  on(l: Listener) {
    listeners.add(l);
    return () => listeners.delete(l);
  },
};
