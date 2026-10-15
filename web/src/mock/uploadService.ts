import type { UploadService } from "@/services/types";
import type { AttachmentKind } from "@/types/models";
import { ServiceError } from "@/services/errors";
import { sleep, uid } from "@/lib/utils";

const MAX = 25 * 1024 * 1024;
const BLOCKED = /\.(exe|bat|cmd|sh|msi|scr|js|vbs|jar)$/i;

const kindOf = (mime: string): AttachmentKind => (mime.startsWith("image/") ? "image" : mime.startsWith("video/") ? "video" : mime.startsWith("audio/") ? "audio" : "file");

export const uploadService: UploadService = {
  async upload(file, onProgress, signal) {
    if (file.size > MAX) throw new ServiceError("file_too_large", "Files can be up to 25 MB.", { status: 413 });
    if (BLOCKED.test(file.name)) throw new ServiceError("file_type", "This file type isn't allowed.", { status: 415 });
    for (let p = 0; p <= 100; p += 20) {
      if (signal?.aborted) throw new ServiceError("aborted", "Upload cancelled");
      onProgress?.(p);
      await sleep(90);
    }
    return { id: uid("att"), kind: kindOf(file.type), name: file.name.slice(0, 120), url: URL.createObjectURL(file), mimeType: file.type || "application/octet-stream", sizeBytes: file.size };
  },
};
