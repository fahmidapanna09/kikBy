import type { ID, ISODate, User } from "@/types/models";

export const cn = (...parts: Array<string | false | null | undefined>): string => parts.filter(Boolean).join(" ");

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

/** Stable 1–6 palette index from any string id. */
export function toneFor(id: string): 1 | 2 | 3 | 4 | 5 | 6 {
  let h = 0;
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0;
  return ((h % 6) + 1) as 1 | 2 | 3 | 4 | 5 | 6;
}

export function uid(prefix = "id"): string {
  const rnd = typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `${Date.now()}${Math.random()}`;
  return `${prefix}_${rnd.replace(/-/g, "").slice(0, 16)}`;
}

const sameDay = (a: Date, b: Date) => a.toDateString() === b.toDateString();

/** "14:05" */
export function formatTime(iso: ISODate): string {
  return new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

/** Chat-list style: time today, "Yesterday", weekday this week, else short date. */
export function formatListTime(iso: ISODate, now = new Date()): string {
  const d = new Date(iso);
  if (sameDay(d, now)) return formatTime(iso);
  const y = new Date(now);
  y.setDate(now.getDate() - 1);
  if (sameDay(d, y)) return "Yesterday";
  const diffDays = (now.getTime() - d.getTime()) / 86400000;
  if (diffDays < 7) return d.toLocaleDateString([], { weekday: "short" });
  return d.toLocaleDateString([], { month: "short", day: "numeric" });
}

/** Date separator label inside a conversation. */
export function formatDayLabel(iso: ISODate, now = new Date()): string {
  const d = new Date(iso);
  if (sameDay(d, now)) return "Today";
  const y = new Date(now);
  y.setDate(now.getDate() - 1);
  if (sameDay(d, y)) return "Yesterday";
  return d.toLocaleDateString([], { weekday: "long", month: "short", day: "numeric", year: d.getFullYear() === now.getFullYear() ? undefined : "numeric" });
}

export function formatLastSeen(user: Pick<User, "presence" | "lastSeenAt">): string {
  if (user.presence === "online") return "online";
  if (!user.lastSeenAt) return "offline";
  const mins = Math.floor((Date.now() - new Date(user.lastSeenAt).getTime()) / 60000);
  if (Number.isFinite(mins) && mins >= 0) {
    if (mins < 1) return "active just now";
    if (mins < 60) return `active ${mins} min ago`;
    if (mins < 24 * 60) return `active ${Math.floor(mins / 60)} h ago`;
  }
  return `last seen ${formatListTime(user.lastSeenAt).toLowerCase()}`;
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1048576) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1048576).toFixed(1)} MB`;
}

export function debounce<A extends unknown[]>(fn: (...a: A) => void, ms: number): ((...a: A) => void) & { cancel: () => void } {
  let t: ReturnType<typeof setTimeout> | undefined;
  const d = (...a: A) => {
    if (t) clearTimeout(t);
    t = setTimeout(() => fn(...a), ms);
  };
  d.cancel = () => t && clearTimeout(t);
  return d;
}

export const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

export function groupBy<T>(items: T[], key: (t: T) => string): Record<string, T[]> {
  const out: Record<string, T[]> = {};
  for (const it of items) (out[key(it)] ||= []).push(it);
  return out;
}

export const byId = <T extends { id: ID }>(items: T[]): Record<ID, T> => Object.fromEntries(items.map((i) => [i.id, i]));

/**
 * User content is rendered ONLY as React text nodes (auto-escaped) — never as HTML.
 * This helper additionally strips control characters / zero-width tricks and caps length,
 * and is applied to every string that comes from a form or the network before display/storage.
 */
export function sanitizeText(input: string, max = 4000): string {
  // eslint-disable-next-line no-control-regex
  return input.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F​-‏‪-‮⁦-⁩]/g, "").slice(0, max);
}

/** Only http(s)/blob/data:image URLs may be used for <img src>/<a href>. Blocks javascript: etc. */
export function safeUrl(url: string | undefined): string | undefined {
  if (!url) return undefined;
  return /^(https?:|blob:|data:image\/(png|jpe?g|gif|webp);)/i.test(url) || /^\/(?!\/)[\w\-./%?=&]*$/.test(url) ? url : undefined; // + same-origin paths (profile-picture links)
}

/** Split text into plain / URL parts so links can be rendered as safe <a> elements. */
export function linkify(text: string): Array<{ text: string; href?: string }> {
  const out: Array<{ text: string; href?: string }> = [];
  const re = /\bhttps?:\/\/[^\s<>"']+/gi;
  let last = 0;
  for (let m = re.exec(text); m; m = re.exec(text)) {
    if (m.index > last) out.push({ text: text.slice(last, m.index) });
    out.push({ text: m[0], href: m[0] });
    last = m.index + m[0].length;
  }
  if (last < text.length) out.push({ text: text.slice(last) });
  return out;
}

/** Short preview text for a message that has attachments but no text (chat list, reply snippets). */
export function attachmentLabel(kind: string | undefined): string {
  switch (kind) {
    case "image": return "Photo";
    case "video": return "Video";
    case "audio": return "Audio";
    default: return "File";
  }
}
