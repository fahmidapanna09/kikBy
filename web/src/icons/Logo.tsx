import { useId } from "react";

/** KikBite mark: chat bubble with a bite taken out of the top-right corner. */
export function LogoMark({ size = 36 }: { size?: number }) {
  const uid = useId().replace(/:/g, "");
  return (
    <svg width={size} height={size} viewBox="0 0 100 100" aria-hidden="true" focusable="false">
      <defs>
        <linearGradient id={`g${uid}`} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#6D4AFF" />
          <stop offset="1" stopColor="#4D8DFF" />
        </linearGradient>
        <mask id={`m${uid}`}>
          <rect width="100" height="100" fill="#fff" />
          <circle cx="76" cy="27" r="16" fill="#000" />
        </mask>
      </defs>
      <path
        mask={`url(#m${uid})`}
        fill={`url(#g${uid})`}
        d="M50 12c21 0 38 15 38 34 0 16-12 29-29 33l-10 12c-1.3 1.5-3.6.7-3.8-1.3l-1-14C27 72 12 59 12 44 12 25 29 12 50 12z"
      />
    </svg>
  );
}

export function Logo({ size = 36, tagline = false }: { size?: number; tagline?: boolean }) {
  return (
    <div className="logo">
      <LogoMark size={size} />
      <div>
        <div className="word">
          Kik<b>Bite</b>
        </div>
        {tagline && <div style={{ font: "600 11.5px var(--font-body)", color: "var(--text-dim)" }}>Small bites. Big conversations.</div>}
      </div>
    </div>
  );
}
