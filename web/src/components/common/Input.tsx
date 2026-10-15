import { useId, useState } from "react";
import type { ReactNode } from "react";
import { Icon } from "@/icons/Icon";
import type { IconName } from "@/icons/Icon";
import { cn } from "@/lib/utils";

interface InputProps {
  label?: string;
  value: string;
  onChange: (value: string) => void;
  onBlur?: () => void;
  type?: "text" | "email" | "password" | "search" | "tel";
  placeholder?: string;
  icon?: IconName;
  error?: string;
  hint?: string;
  autoComplete?: string;
  autoFocus?: boolean;
  disabled?: boolean;
  name?: string;
  maxLength?: number;
  inputMode?: "text" | "email" | "search" | "numeric" | "tel";
  trailing?: ReactNode;
  "aria-label"?: string;
  onKeyDown?: (e: any) => void;
  inputRef?: { current: HTMLInputElement | null };
}

/** Labelled input with inline error (aria-invalid + aria-describedby) and password show/hide. */
export function Input({ label, value, onChange, onBlur, type = "text", icon, error, hint, trailing, inputRef, ...rest }: InputProps) {
  const id = useId();
  const [shown, setShown] = useState(false);
  const isPw = type === "password";
  const msgId = `${id}-msg`;
  return (
    <div className="field">
      {label && (
        <label className="field-label" htmlFor={id}>
          {label}
        </label>
      )}
      <div className={cn("input-wrap", !!error && "invalid")}>
        {icon && <Icon name={icon} />}
        <input
          id={id}
          ref={inputRef}
          className="input"
          type={isPw && shown ? "text" : type}
          value={value}
          onChange={(e: any) => onChange(e.target.value)}
          onBlur={onBlur}
          aria-invalid={error ? true : undefined}
          aria-describedby={error || hint ? msgId : undefined}
          {...rest}
        />
        {trailing}
        {isPw && (
          <button type="button" className="icon-btn" style={{ width: 32, height: 32 }} onClick={() => setShown((s) => !s)} aria-label={shown ? "Hide password" : "Show password"} aria-pressed={shown}>
            <Icon name={shown ? "eye-off" : "eye"} size={18} />
          </button>
        )}
      </div>
      {error ? (
        <div id={msgId} className="field-error" role="alert">
          {error}
        </div>
      ) : hint ? (
        <div id={msgId} className="field-hint">
          {hint}
        </div>
      ) : null}
    </div>
  );
}

export function Checkbox({ checked, onChange, children }: { checked: boolean; onChange: (v: boolean) => void; children: ReactNode }) {
  return (
    <label className="check">
      <input type="checkbox" checked={checked} onChange={(e: any) => onChange(e.target.checked)} />
      <span className="box">
        <Icon name="check" />
      </span>
      <span>{children}</span>
    </label>
  );
}

export function Switch({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <span className="switch">
      <input type="checkbox" role="switch" checked={checked} onChange={(e: any) => onChange(e.target.checked)} aria-label={label} />
      <span className="track" aria-hidden="true" />
    </span>
  );
}
