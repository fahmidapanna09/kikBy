import { useCallback, useEffect, useRef } from "react";
import { sendRealtime } from "@/realtime";
import { useApp } from "@/state/store";

/** Emits typing:start (throttled to once per 2.5s) and typing:stop (after 3s idle / on send). Respects the privacy setting. */
export function useTypingEmitter(conversationId: string) {
  const enabled = useApp((s) => s.settings?.privacy.typingIndicator ?? true);
  const last = useRef(0);
  const stopTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const active = useRef(false);

  const stop = useCallback(() => {
    if (stopTimer.current) clearTimeout(stopTimer.current);
    if (active.current) sendRealtime({ type: "typing:stop", conversationId });
    active.current = false;
  }, [conversationId]);

  const typed = useCallback(() => {
    if (!enabled) return;
    const now = Date.now();
    if (now - last.current > 2500) {
      sendRealtime({ type: "typing:start", conversationId });
      last.current = now;
      active.current = true;
    }
    if (stopTimer.current) clearTimeout(stopTimer.current);
    stopTimer.current = setTimeout(stop, 3000);
  }, [enabled, conversationId, stop]);

  useEffect(() => stop, [stop]);
  return { typed, stop };
}
