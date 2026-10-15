import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Icon } from "@/icons/Icon";
import type { IconName } from "@/icons/Icon";
import { cn } from "@/lib/utils";

export interface MenuEntry {
  label: string;
  icon?: IconName;
  danger?: boolean;
  onSelect: () => void;
  hidden?: boolean;
  separatorBefore?: boolean;
}

/** Positioned menu with arrow-key navigation; closes on Esc / outside click / scroll. */
export function Menu({ items, x, y, onClose, header }: { items: MenuEntry[]; x: number; y: number; onClose: () => void; header?: ReactNode }) {
  const ref = useRef<HTMLDivElement | null>(null);
  const [pos, setPos] = useState({ left: x, top: y });
  const visible = items.filter((i) => !i.hidden);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    setPos({
      left: Math.max(8, Math.min(x, window.innerWidth - r.width - 8)),
      top: Math.max(8, Math.min(y, window.innerHeight - r.height - 8)),
    });
    el.querySelector<HTMLElement>("button")?.focus();
  }, [x, y]);

  useEffect(() => {
    const down = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        const btns = Array.from(ref.current?.querySelectorAll<HTMLElement>("button") ?? []);
        const i = btns.indexOf(document.activeElement as HTMLElement);
        const next = e.key === "ArrowDown" ? (i + 1) % btns.length : (i - 1 + btns.length) % btns.length;
        btns[next]?.focus();
      }
    };
    document.addEventListener("mousedown", down);
    document.addEventListener("keydown", key);
    window.addEventListener("resize", onClose);
    window.addEventListener("scroll", onClose, true);
    return () => {
      document.removeEventListener("mousedown", down);
      document.removeEventListener("keydown", key);
      window.removeEventListener("resize", onClose);
      window.removeEventListener("scroll", onClose, true);
    };
  }, [onClose]);

  return (
    <div className="menu" role="menu" ref={ref} style={pos}>
      {header}
      {visible.map((it, i) => (
        <div key={it.label}>
          {it.separatorBefore && i > 0 && <div className="menu-sep" role="separator" />}
          <button
            type="button"
            role="menuitem"
            className={cn("menu-item", it.danger && "danger")}
            onClick={() => {
              onClose();
              it.onSelect();
            }}
          >
            {it.icon && <Icon name={it.icon} />}
            {it.label}
          </button>
        </div>
      ))}
    </div>
  );
}

/** Trigger + menu in one: <Dropdown trigger={...} items={[...]} /> */
export function Dropdown({ label, items, children }: { label: string; items: MenuEntry[]; children: ReactNode }) {
  const [at, setAt] = useState<{ x: number; y: number } | null>(null);
  return (
    <>
      <button
        type="button"
        className="icon-btn"
        aria-label={label}
        title={label}
        aria-haspopup="menu"
        aria-expanded={!!at}
        onClick={(e: any) => {
          const r = e.currentTarget.getBoundingClientRect();
          setAt({ x: r.right - 188, y: r.bottom + 6 });
        }}
      >
        {children}
      </button>
      {at && <Menu items={items} x={at.x} y={at.y} onClose={() => setAt(null)} />}
    </>
  );
}
