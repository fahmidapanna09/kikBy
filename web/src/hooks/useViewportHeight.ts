import { useEffect } from "react";

/**
 * Mobile keyboard safety: when the on-screen keyboard opens, visualViewport shrinks while the layout
 * viewport may not (iOS). Publish the visible height as --vvh so the shell (and thus the composer) stays above the keyboard.
 */
export function useViewportHeight() {
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    const apply = () => {
      document.documentElement.style.setProperty("--vvh", `${vv.height}px`);
      if (vv.offsetTop) window.scrollTo(0, 0);
    };
    apply();
    vv.addEventListener("resize", apply);
    vv.addEventListener("scroll", apply);
    return () => {
      vv.removeEventListener("resize", apply);
      vv.removeEventListener("scroll", apply);
      document.documentElement.style.removeProperty("--vvh");
    };
  }, []);
}
