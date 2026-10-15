import { memo, useState } from "react";
import type { Presence } from "@/types/models";
import { initials, safeUrl, toneFor } from "@/lib/utils";
import { avatarLink } from "@/api/mappers";
import { config } from "@/services/config";

interface AvatarProps {
  name: string;
  id?: string;
  src?: string;
  size?: number;
  group?: boolean;
  presence?: Presence;
  tone?: 1 | 2 | 3 | 4 | 5 | 6;
}

/** Ids whose picture request already answered 404 in this session: no point asking again on every render. */
const noPicture = new Set<string>();

export const Avatar = memo(function Avatar({ name, id, src, size = 40, group, presence, tone }: AvatarProps) {
  const t = tone ?? toneFor(id ?? name);
  const [failed, setFailed] = useState<string | undefined>();
  // Live accounts: when the caller has no picture URL (friend rows, chat lists…) ask the server; a 404 falls back to initials.
  const auto = !src && !group && id && config.liveAuth && !noPicture.has(id) ? avatarLink(id) : undefined;
  const candidate = safeUrl(src) ?? auto;
  const url = candidate && candidate !== failed ? candidate : undefined; // a picture that failed to load shows initials instead of a broken image
  return (
    <span className={`avatar av-${t}${group ? " group" : ""}`} style={{ width: size, height: size, fontSize: Math.max(11, Math.round(size * 0.34)) }}>
      {url ? (
        <img
          src={url}
          alt=""
          loading="lazy"
          decoding="async"
          onError={() => {
            if (id && url === auto) noPicture.add(id);
            setFailed(url);
          }}
        />
      ) : (
        <span aria-hidden="true">{initials(name)}</span>
      )}
      {presence && <span className={`presence ${presence}`} role="img" aria-label={presence} />}
    </span>
  );
});
