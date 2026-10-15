import type { ChangeEvent, FocusEvent, KeyboardEvent } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/common/Button";
import { Icon } from "@/icons/Icon";
import { navigate } from "@/lib/router";
import { errorMessage, isServiceError, services, useSession } from "@/services";
import { AuthLayout } from "./AuthLayout";

const LEN = 6;
const COOLDOWN = 60; // seconds; mirrors the server's resend cooldown

/** Enter the 6-digit code e-mailed to the signed-in user (valid for 10 minutes). */
export default function VerifyEmailPage() {
  const user = useSession()?.user;
  const [digits, setDigits] = useState<string[]>(Array(LEN).fill(""));
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState("");
  const [note, setNote] = useState("");
  const [wait, setWait] = useState(0);
  const cur = useRef<string[]>(Array(LEN).fill("")); // always-current copy, so fast typing/paste never reads stale state
  const working = useRef(false); // true while a code is being checked: the profile refresh must not bounce us home before "verified" shows
  const refs = useRef<Array<HTMLInputElement | null>>([]);
  const target = user?.pendingEmail || user?.email || "your e-mail";

  useEffect(() => {
    if (wait <= 0) return;
    const t = setTimeout(() => setWait((w) => w - 1), 1000);
    return () => clearTimeout(t);
  }, [wait]);

  // Already verified (and no pending change): nothing to do here.
  useEffect(() => {
    if (user && user.emailVerified !== false && !user.pendingEmail && !done && !working.current) navigate("/", { replace: true });
  }, [user, done]);

  const submit = useCallback(async (code: string) => {
    setBusy(true);
    setError("");
    setNote("");
    working.current = true;
    try {
      await services.auth.verifyEmail(code);
      setDone(true);
    } catch (e) {
      setError(errorMessage(e, "Couldn't verify the code."));
      if (!isServiceError(e) || e.code === "invalid_code") {
        reset();
        refs.current[0]?.focus();
      }
    } finally {
      working.current = false;
      setBusy(false);
    }
  }, []);

  const reset = () => {
    cur.current = Array(LEN).fill("");
    setDigits(cur.current);
  };

  const apply = (start: number, raw: string) => {
    const chars = raw.replace(/\D/g, "").slice(0, LEN - start).split("");
    if (!chars.length) return;
    const next = [...cur.current];
    chars.forEach((c, i) => (next[start + i] = c));
    cur.current = next;
    setDigits(next);
    setError("");
    const last = Math.min(start + chars.length, LEN - 1);
    refs.current[last]?.focus();
    if (next.every(Boolean)) void submit(next.join(""));
  };

  const onKey = (i: number, e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Backspace" && !cur.current[i] && i > 0) {
      const next = [...cur.current];
      next[i - 1] = "";
      cur.current = next;
      setDigits(next);
      refs.current[i - 1]?.focus();
    } else if (e.key === "ArrowLeft" && i > 0) refs.current[i - 1]?.focus();
    else if (e.key === "ArrowRight" && i < LEN - 1) refs.current[i + 1]?.focus();
  };

  const resend = async () => {
    setError("");
    setNote("");
    try {
      const r = await services.auth.resendVerification();
      if (r.alreadyVerified) {
        void services.users.me().catch(() => undefined);
        setDone(true);
        return;
      }
      reset();
      refs.current[0]?.focus();
      setNote(`New code sent to ${target}.`);
      setWait(COOLDOWN);
    } catch (e) {
      setError(errorMessage(e, "Couldn't send a new code."));
    }
  };

  if (done) {
    return (
      <AuthLayout title="E-mail verified" subtitle="Thanks — your address is confirmed.">
        <div className="form-ok" role="status"><Icon name="check" size={16} /> All set.</div>
        <Button block onClick={() => navigate("/", { replace: true })}>Continue to KikBite</Button>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout title="Verify your e-mail" subtitle={`Enter the 6-digit code we sent to ${target}. It expires in 10 minutes.`}>
      <div className="code-row" role="group" aria-label="6-digit verification code">
        {digits.map((d, i) => (
          <input
            key={i}
            ref={(el: HTMLInputElement | null) => { refs.current[i] = el; }}
            className="code-box"
            value={d}
            inputMode="numeric"
            pattern="[0-9]*"
            autoComplete={i === 0 ? "one-time-code" : "off"}
            maxLength={LEN}
            autoFocus={i === 0}
            disabled={busy}
            aria-label={`Digit ${i + 1}`}
            onChange={(e: ChangeEvent<HTMLInputElement>) => apply(i, e.target.value)}
            onKeyDown={(e: KeyboardEvent<HTMLInputElement>) => onKey(i, e)}
            onFocus={(e: FocusEvent<HTMLInputElement>) => e.target.select()}
          />
        ))}
      </div>
      {error && <div className="form-error" role="alert">{error}</div>}
      {note && <div className="form-ok" role="status" aria-live="polite">{note}</div>}
      <Button block loading={busy} disabled={digits.some((d) => !d)} onClick={() => void submit(digits.join(""))}>Verify</Button>
      <p className="field-hint" style={{ textAlign: "center" }}>
        Didn't get it? Check your spam folder, or{" "}
        <button type="button" className="banner-action" style={{ marginLeft: 0 }} onClick={resend} disabled={wait > 0 || busy}>
          {wait > 0 ? `resend in ${wait}s` : "send a new code"}
        </button>
      </p>
      <p className="field-hint" style={{ textAlign: "center" }}>
        <button type="button" className="banner-action" style={{ marginLeft: 0 }} onClick={() => navigate("/", { replace: true })}>Do this later</button>
      </p>
    </AuthLayout>
  );
}
