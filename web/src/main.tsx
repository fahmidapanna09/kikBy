import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "@/app/App";
import { ErrorBoundary } from "@/app/ErrorBoundary";
import { ToastRegion } from "@/components/common/Toast";

createRoot(document.getElementById("root") as HTMLElement).render(
  <StrictMode>
    <ErrorBoundary>
      <a className="skip-link" href="#main">Skip to content</a>
      <App />
      <ToastRegion />
    </ErrorBoundary>
  </StrictMode>,
);
