import { useEffect, useId, useRef } from "react";
import type { ReactNode } from "react";
import { Icon } from "@/icons/Icon";
import { Button } from "./Button";

const FOCUSABLE = 'a[href],button:not([disabled]),input:not([disabled]),textarea:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])';

/** Esc to close, focus trap, focus restore, scroll-safe backdrop click. */
function useDialogBehavior(onClose: () => void) {
  const ref = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    const el = ref.current;
    const first = el?.querySelector<HTMLElement>("[data-autofocus]") ?? el?.querySelector<HTMLElement>(FOCUSABLE);
    (first ?? el)?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
      } else if (e.key === "Tab" && el) {
        const items = Array.from(el.querySelectorAll<HTMLElement>(FOCUSABLE));
        if (items.length === 0) return;
        const a = items[0];
        const z = items[items.length - 1];
        if (e.shiftKey && document.activeElement === a) {
          e.preventDefault();
          z.focus();
        } else if (!e.shiftKey && document.activeElement === z) {
          e.preventDefault();
          a.focus();
        }
      }
    };
    document.addEventListener("keydown", onKey, true);
    return () => {
      document.removeEventListener("keydown", onKey, true);
      prev?.focus?.();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return ref;
}

interface DialogProps {
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
}

export function Modal({ title, onClose, children, footer }: DialogProps) {
  const ref = useDialogBehavior(onClose);
  const tid = useId();
  return (
    <div className="scrim" onMouseDown={(e: any) => e.target === e.currentTarget && onClose()}>
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby={tid} ref={ref} tabIndex={-1}>
        <div className="modal-head">
          <h2 id={tid}>{title}</h2>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Close">
            <Icon name="close" />
          </button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-foot">{footer}</div>}
      </div>
    </div>
  );
}

/** Bottom sheet on mobile, side drawer on tablet/desktop. */
export function Drawer({ title, onClose, children }: Omit<DialogProps, "footer">) {
  const ref = useDialogBehavior(onClose);
  const tid = useId();
  return (
    <div className="scrim drawer" onMouseDown={(e: any) => e.target === e.currentTarget && onClose()}>
      <div className="drawer-sheet" role="dialog" aria-modal="true" aria-labelledby={tid} ref={ref} tabIndex={-1}>
        <div className="grab" />
        <div className="modal-head">
          <h2 id={tid}>{title}</h2>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Close">
            <Icon name="close" />
          </button>
        </div>
        <div className="modal-body" style={{ flex: 1 }}>
          {children}
        </div>
      </div>
    </div>
  );
}

interface ConfirmProps {
  title: string;
  message: string;
  confirmLabel?: string;
  danger?: boolean;
  onConfirm: () => void | Promise<void>;
  onClose: () => void;
}

export function ConfirmDialog({ title, message, confirmLabel = "Confirm", danger, onConfirm, onClose }: ConfirmProps) {
  return (
    <Modal
      title={title}
      onClose={onClose}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant={danger ? "danger" : "primary"}
            autoFocus
            onClick={async () => {
              await onConfirm();
              onClose();
            }}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      <p>{message}</p>
    </Modal>
  );
}
