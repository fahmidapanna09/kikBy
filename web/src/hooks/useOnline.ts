import { useEffect } from "react";
import { store, useApp } from "@/state/store";

/** Mirrors navigator.onLine into the store so any component can show offline UI. */
export function useNetworkWatcher() {
  useEffect(() => {
    const on = () => store.set({ online: true });
    const off = () => store.set({ online: false });
    window.addEventListener("online", on);
    window.addEventListener("offline", off);
    return () => {
      window.removeEventListener("online", on);
      window.removeEventListener("offline", off);
    };
  }, []);
}
export const useOnline = () => useApp((s) => s.online);
