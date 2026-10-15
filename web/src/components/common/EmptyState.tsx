import type { ReactNode } from "react";
import { Icon } from "@/icons/Icon";
import type { IconName } from "@/icons/Icon";
import { Button } from "./Button";

interface EmptyStateProps {
  icon: IconName;
  title: string;
  message?: string;
  actions?: ReactNode;
}

export function EmptyState({ icon, title, message, actions }: EmptyStateProps) {
  return (
    <div className="empty">
      <div className="blob">
        <Icon name={icon} />
      </div>
      <h3>{title}</h3>
      {message && <p>{message}</p>}
      {actions && <div className="actions">{actions}</div>}
    </div>
  );
}

/** Standard error state with retry. */
export function ErrorState({ message = "Something went wrong.", onRetry }: { message?: string; onRetry?: () => void }) {
  return (
    <div className="empty" role="alert">
      <div className="blob" style={{ color: "var(--error)" }}>
        <Icon name="alert" />
      </div>
      <h3>Couldn't load this</h3>
      <p>{message}</p>
      {onRetry && (
        <div className="actions">
          <Button variant="secondary" size="sm" onClick={onRetry}>
            <Icon name="refresh" size={16} /> Try again
          </Button>
        </div>
      )}
    </div>
  );
}
