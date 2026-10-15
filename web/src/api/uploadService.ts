import type { UploadService } from "@/services/types";
import type { Attachment, AttachmentKind } from "@/types/models";
import { ServiceError } from "@/services/errors";
import { http } from "@/services/http";

/** Mirrors the backend allow-list (media.allowedMimeTypes) so obvious mistakes fail before any bytes are sent. */
const ALLOWED: Record<string, { kind: "image" | "video" | "audio" | "document" | "voice"; ui: AttachmentKind }> = {
  "image/jpeg": { kind: "image", ui: "image" },
  "image/png": { kind: "image", ui: "image" },
  "image/webp": { kind: "image", ui: "image" },
  "image/gif": { kind: "image", ui: "image" },
  "video/mp4": { kind: "video", ui: "video" },
  "video/webm": { kind: "video", ui: "video" },
  "video/3gpp": { kind: "video", ui: "video" },
  "audio/mpeg": { kind: "audio", ui: "audio" },
  "audio/mp4": { kind: "audio", ui: "audio" },
  "audio/aac": { kind: "audio", ui: "audio" },
  "audio/webm": { kind: "audio", ui: "audio" },
  "application/pdf": { kind: "document", ui: "file" },
  "application/msword": { kind: "document", ui: "file" },
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document": { kind: "document", ui: "file" },
  "text/plain": { kind: "document", ui: "file" },
  "application/zip": { kind: "document", ui: "file" },
};

/** Browsers (Windows especially) sometimes report an empty or non-standard type; fall back on the extension. */
const BY_EXT: Record<string, string> = {
  jpg: "image/jpeg", jpeg: "image/jpeg", png: "image/png", webp: "image/webp", gif: "image/gif",
  mp4: "video/mp4", webm: "video/webm", "3gp": "video/3gpp", mp3: "audio/mpeg", m4a: "audio/mp4", aac: "audio/aac",
  pdf: "application/pdf", doc: "application/msword",
  docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", txt: "text/plain", zip: "application/zip",
};

/** Voice notes use their own kind server-side (different allow-list: ogg/amr/aac/webm/mp4 audio). */
const VOICE_MIMES = new Set(["audio/webm", "audio/ogg", "audio/mp4", "audio/aac", "audio/amr"]);

const LIMITS: Record<string, number> = { image: 15, video: 200, audio: 25, voice: 25, document: 100 }; // MB, matches backend defaults

export function mimeFor(file: Pick<File, "name" | "type">): string {
  const t = (file.type || "").toLowerCase().split(";")[0].trim();
  if (ALLOWED[t]) return t;
  const ext = file.name.includes(".") ? file.name.split(".").pop()!.toLowerCase() : "";
  return BY_EXT[ext] ?? t;
}

interface Presign {
  media_id: string;
  upload_url: string;
}
interface Confirm {
  media_id: string;
  status: string;
  download_url?: string;
  reason?: string;
}

/** PUT with progress (fetch has no upload progress). The Content-Type must equal the one the URL was signed for. */
function putFile(url: string, file: File, mime: string, onProgress?: (pct: number) => void, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const onAbort = () => xhr.abort();
    xhr.open("PUT", url);
    xhr.setRequestHeader("Content-Type", mime);
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress?.(Math.min(99, Math.round((e.loaded / e.total) * 100)));
    };
    xhr.onload = () => {
      signal?.removeEventListener("abort", onAbort);
      if (xhr.status >= 200 && xhr.status < 300) resolve();
      else reject(new ServiceError("upload_failed", "The upload was refused by storage. Please try again.", { status: xhr.status }));
    };
    xhr.onerror = () => {
      signal?.removeEventListener("abort", onAbort);
      reject(new ServiceError("network", "Upload failed. Check your connection and try again."));
    };
    xhr.onabort = () => reject(new ServiceError("aborted", "Upload cancelled"));
    if (signal?.aborted) return reject(new ServiceError("aborted", "Upload cancelled"));
    signal?.addEventListener("abort", onAbort, { once: true });
    xhr.send(file);
  });
}

/* Signed download links expire (15 min server-side), so cache for less than that and re-ask afterwards. */
const URL_TTL_MS = 10 * 60 * 1000;
const thumbCache = new Map<string, { p: Promise<{ url: string; thumb: boolean }>; at: number }>();
const urlCache = new Map<string, { url: Promise<string>; at: number }>();

export const liveUploadService: UploadService = {
  async upload(file, onProgress, signal, opts) {
    const mime = opts?.voice ? (file.type || "").toLowerCase().split(";")[0].trim() : mimeFor(file);
    const info = opts?.voice ? (VOICE_MIMES.has(mime) ? ({ kind: "voice", ui: "audio" } as const) : undefined) : ALLOWED[mime];
    if (!info) throw new ServiceError("file_type", "This file type isn't supported.", { status: 415 });
    const max = LIMITS[info.kind] * 1024 * 1024;
    if (file.size > max) throw new ServiceError("file_too_large", `${info.kind === "document" ? "Files" : info.kind[0].toUpperCase() + info.kind.slice(1) + "s"} can be up to ${LIMITS[info.kind]} MB.`, { status: 413 });
    if (file.size === 0) throw new ServiceError("file_empty", "This file is empty.");

    onProgress?.(0);
    const p = await http<Presign>("/media/presign-upload", { method: "POST", body: { kind: info.kind, mime_type: mime, file_name: file.name }, signal });
    await putFile(p.upload_url, file, mime, onProgress, signal);
    const c = await http<Confirm>(`/media/${encodeURIComponent(p.media_id)}/confirm`, { method: "POST", signal });
    if (c.status !== "uploaded") throw new ServiceError("upload_rejected", c.reason || "This file was rejected.", { status: 422 });
    onProgress?.(100);

    if (c.download_url) urlCache.set(p.media_id, { url: Promise.resolve(c.download_url), at: Date.now() });
    const att: Attachment = { id: p.media_id, kind: info.ui, name: file.name.slice(0, 120), url: c.download_url ?? "", mimeType: mime, sizeBytes: file.size };
    return att;
  },

  resolveThumbnail(id) {
    const hit = thumbCache.get(id);
    if (hit && Date.now() - hit.at < URL_TTL_MS) return hit.p;
    const p = http<{ download_url: string; variant?: string }>(`/media/${encodeURIComponent(id)}/download-url?variant=thumb`).then((r) => ({ url: r.download_url, thumb: r.variant === "thumb" }));
    thumbCache.set(id, { p, at: Date.now() });
    p.catch(() => thumbCache.delete(id));
    return p;
  },

  resolveUrl(id) {
    const hit = urlCache.get(id);
    if (hit && Date.now() - hit.at < URL_TTL_MS) return hit.url;
    const url = http<{ download_url: string }>(`/media/${encodeURIComponent(id)}/download-url`).then((r) => r.download_url);
    urlCache.set(id, { url, at: Date.now() });
    url.catch(() => urlCache.delete(id)); // don't cache a failure
    return url;
  },
};
