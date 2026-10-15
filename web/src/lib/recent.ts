import { useSyncExternalStore } from "react";

/** Recent searches — per browser, capped at 8. Wrapped in try/catch: storage can be unavailable. */
const KEY = "kikbite.recentSearches";
let list: string[] = read();
const subs = new Set<() => void>();
function read(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) ?? "[]");
    return Array.isArray(v) ? v.filter((x) => typeof x === "string").slice(0, 8) : [];
  } catch {
    return [];
  }
}
function write(next: string[]) {
  list = next;
  try {
    localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    /* ignore */
  }
  subs.forEach((s) => s());
}
export const recentSearches = {
  add: (q: string) => {
    const t = q.trim().slice(0, 60);
    if (t.length >= 2) write([t, ...list.filter((x) => x.toLowerCase() !== t.toLowerCase())].slice(0, 8));
  },
  remove: (q: string) => write(list.filter((x) => x !== q)),
  clear: () => write([]),
};
export const useRecentSearches = () => useSyncExternalStore((cb) => (subs.add(cb), () => subs.delete(cb)), () => list);
