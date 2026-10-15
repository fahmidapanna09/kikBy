import { cn } from "@/lib/utils";

export function Skeleton({ w, h = 14, circle, className }: { w?: number | string; h?: number | string; circle?: boolean; className?: string }) {
  return <span className={cn("skeleton", circle && "circle", className)} style={{ width: w ?? "100%", height: h, display: "block" }} aria-hidden="true" />;
}

/** Skeleton rows matching a chat-list / contact row. */
export function ListSkeleton({ rows = 7 }: { rows?: number }) {
  return (
    <div role="status" aria-label="Loading" style={{ padding: "8px 16px", display: "flex", flexDirection: "column", gap: 18 }}>
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} style={{ display: "flex", gap: 12, alignItems: "center" }}>
          <Skeleton w={48} h={48} circle />
          <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 8 }}>
            <Skeleton w={`${50 + ((i * 13) % 35)}%`} h={13} />
            <Skeleton w={`${30 + ((i * 17) % 45)}%`} h={11} />
          </div>
        </div>
      ))}
      <span className="sr-only">Loading…</span>
    </div>
  );
}
