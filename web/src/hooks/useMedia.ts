import { useSyncExternalStore } from "react";

export function useMedia(query: string): boolean {
  return useSyncExternalStore(
    (cb) => {
      const m = window.matchMedia(query);
      m.addEventListener("change", cb);
      return () => m.removeEventListener("change", cb);
    },
    () => window.matchMedia(query).matches,
    () => false,
  );
}
/** Breakpoints from the spec: mobile <768, tablet 768–1279, desktop ≥1280. */
export const useIsMobile = () => useMedia("(max-width: 767px)");
export const useIsDesktop = () => useMedia("(min-width: 1280px)");
