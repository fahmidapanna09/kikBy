import { useEffect, useState } from "react";
import { Icon } from "@/icons/Icon";
import { navigate } from "@/lib/router";
import { errorMessage, services, useSession } from "@/services";

/**
 * Shown while the live account's e-mail is unverified. Not a gate: nothing is blocked, it only nudges.
 * Coming back to the tab re-reads the profile, so verifying in another tab/device clears it.
 */
export function VerifyEmailBanner() {
  const me = useSession()?.user;
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const unverified = me?.emailVerified === false;

  useEffect(() => {
    if (!unverified) return;
    const onVisible = () => document.visibilityState === "visible" && void services.users.me().catch(() => undefined);
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [unverified]);

  if (!unverified) return null;

  const resend = async () => {
    setBusy(true);
    setNote("");
    try {
      const r = await services.auth.resendVerification();
      setNote(r.alreadyVerified ? "Already verified." : `New code sent — check ${me?.email ?? "your inbox"}.`);
      if (r.alreadyVerified) void services.users.me().catch(() => undefined);
    } catch (e) {
      setNote(errorMessage(e, "Couldn't send the code."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="banner info" role="status">
      <Icon name="mail" size={16} />
      <span>Verify your e-mail address{me?.email ? ` (${me.email})` : ""}.</span>
      <button type="button" className="banner-action" onClick={() => navigate("/verify-email")}>Enter code</button>
      <button type="button" className="banner-action" style={{ marginLeft: 0 }} onClick={resend} disabled={busy}>{busy ? "Sending…" : "Resend code"}</button>
      {note && <span aria-live="polite">{note}</span>}
    </div>
  );
}
