import type { ReactNode } from "react";
import { Logo } from "@/icons/Logo";

export function AuthLayout({ title, subtitle, children }: { title: string; subtitle?: string; children: ReactNode }) {
  return (
    <main className="auth-page" id="main">
      <div className="auth-card">
        <Logo size={40} tagline />
        <div>
          <h1>{title}</h1>
          {subtitle && <p className="sub" style={{ marginTop: 8 }}>{subtitle}</p>}
        </div>
        {children}
      </div>
    </main>
  );
}

/** Only same-origin absolute paths may be used as post-login redirect targets (no open redirect). */
export function safeNext(next: string | null): string {
  if (next && next.startsWith("/") && !next.startsWith("//") && !next.startsWith("/login") && !next.startsWith("/register")) return next;
  return "/";
}
