import type { Settings } from "@/types/models";

/** Applies appearance settings to <html>; returns a cleanup. Dark theme is the only theme (per design). */
export function applyAppearance(a?: Settings["appearance"]): void {
  const el = document.documentElement;
  el.style.setProperty("--font-scale", String((a?.fontScale ?? 100) / 100));
  el.dataset.density = a?.density ?? "comfortable";
  el.dataset.reduceMotion = String(!!a?.reduceMotion);
}
