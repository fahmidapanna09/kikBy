import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

interface ButtonProps {
  children?: ReactNode;
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "md" | "sm";
  block?: boolean;
  loading?: boolean;
  disabled?: boolean;
  type?: "button" | "submit";
  onClick?: (e: any) => void;
  className?: string;
  "aria-label"?: string;
  autoFocus?: boolean;
}

export function Button({ children, variant = "primary", size = "md", block, loading, disabled, type = "button", className, ...rest }: ButtonProps) {
  return (
    <button
      type={type}
      className={cn("btn", `btn-${variant}`, size === "sm" && "btn-sm", block && "btn-block", className)}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      {...rest}
    >
      {loading && <span className="spinner" aria-hidden="true" />}
      {children}
    </button>
  );
}

interface IconButtonProps {
  label: string;
  children: ReactNode;
  onClick?: (e: any) => void;
  active?: boolean;
  disabled?: boolean;
  large?: boolean;
  className?: string;
  badge?: boolean;
  "aria-expanded"?: boolean;
  "aria-haspopup"?: "menu" | "dialog";
  type?: "button" | "submit";
}

/** Icon-only buttons MUST have an accessible label. */
export function IconButton({ label, children, active, large, badge, className, ...rest }: IconButtonProps) {
  return (
    <button type="button" className={cn("icon-btn", large && "icon-btn-lg", active && "active", className)} aria-label={label} title={label} {...rest}>
      {children}
      {badge && <span className="badge-dot" aria-hidden="true" />}
    </button>
  );
}
