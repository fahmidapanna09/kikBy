import { useSyncExternalStore } from "react";
import { Icon } from "@/icons/Icon";
import { uid } from "@/lib/utils";

export interface ToastItem {
  id: string;
  kind: "success" | "error" | "info";
  message: string;
  actionLabel?: string;
  onAction?: () => void;
}

let toasts: ToastItem[] = [];
const subs = new Set<() => void>();
const emit = () => subs.forEach((s) => s());

export function dismissToast(id: string) {
  toasts = toasts.filter((t) => t.id !== id);
  emit();
}

function push(kind: ToastItem["kind"], message: string, opts: Partial<Pick<ToastItem, "actionLabel" | "onAction">> = {}) {
  const id = uid("toast");
  toasts = [...toasts.slice(-3), { id, kind, message, ...opts }];
  emit();
  setTimeout(() => dismissToast(id), opts.onAction ? 7000 : 4000);
}

export const toast = {
  success: (m: string, o?: Partial<Pick<ToastItem, "actionLabel" | "onAction">>) => push("success", m, o),
  error: (m: string, o?: Partial<Pick<ToastItem, "actionLabel" | "onAction">>) => push("error", m, o),
  info: (m: string, o?: Partial<Pick<ToastItem, "actionLabel" | "onAction">>) => push("info", m, o),
};

const ICONS = { success: "check", error: "alert", info: "info" } as const;

export function ToastRegion() {
  const list = useSyncExternalStore(
    (cb) => {
      subs.add(cb);
      return () => subs.delete(cb);
    },
    () => toasts,
  );
  return (
    <div className="toast-region" aria-live="polite" aria-atomic="false">
      {list.map((t) => (
        <div key={t.id} className={`toast ${t.kind}`} role={t.kind === "error" ? "alert" : "status"}>
          <Icon name={ICONS[t.kind]} />
          <span className="t-msg">{t.message}</span>
          {t.actionLabel && (
            <button
              type="button"
              className="t-action"
              onClick={() => {
                t.onAction?.();
                dismissToast(t.id);
              }}
            >
              {t.actionLabel}
            </button>
          )}
          <button type="button" className="icon-btn" style={{ width: 28, height: 28 }} aria-label="Dismiss notification" onClick={() => dismissToast(t.id)}>
            <Icon name="close" size={14} />
          </button>
        </div>
      ))}
    </div>
  );
}
