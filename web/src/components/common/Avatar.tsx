import { memo } from "react";
import type { Presence } from "@/types/models";
import { initials, safeUrl, toneFor } from "@/lib/utils";

interface AvatarProps {
  name: string;
  id?: string;
  src?: string;
  size?: number;
  group?: boolean;
  presence?: Presence;
  tone?: 1 | 2 | 3 | 4 | 5 | 6;
}

export const Avatar = memo(function Avatar({ name, id, src, size = 40, group, presence, tone }: AvatarProps) {
  const t = tone ?? toneFor(id ?? name);
  const url = safeUrl(src);
  return (
    <span className={`avatar av-${t}${group ? " group" : ""}`} style={{ width: size, height: size, fontSize: Math.max(11, Math.round(size * 0.34)) }}>
      {url ? <img src={url} alt="" loading="lazy" decoding="async" /> : <span aria-hidden="true">{initials(name)}</span>}
      {presence && <span className={`presence ${presence}`} role="img" aria-label={presence} />}
    </span>
  );
});
