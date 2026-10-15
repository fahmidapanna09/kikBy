import { Component } from "react";
import type { ReactNode } from "react";
import { ErrorState } from "@/components/common/EmptyState";

/** Last-resort UI guard: a render error shows a recoverable state instead of a blank screen. */
export class ErrorBoundary extends Component<{ children?: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidCatch(error: unknown) {
    console.error("UI error:", error);
  }
  render() {
    if (this.state.failed) return <ErrorState message="An unexpected error occurred." onRetry={() => window.location.reload()} />;
    return this.props.children;
  }
}
