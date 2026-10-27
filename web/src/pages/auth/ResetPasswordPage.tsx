import { useEffect, useState } from "react";
import { Button } from "@/components/common/Button";
import { Input } from "@/components/common/Input";
import { Icon } from "@/icons/Icon";
import { Link, navigate, useLocation } from "@/lib/router";
import { passwordStrength, validateConfirm, validatePassword } from "@/lib/validation";
import { config } from "@/services/config";
import { errorMessage, mockLastResetCode, services } from "@/services";
import { toast } from "@/components/common/Toast";
import { AuthLayout } from "./AuthLayout";

export default function ResetPasswordPage() {
  const { query } = useLocation();
  const email = (query.get("email") ?? "").trim();
  const [code, setCode] = useState("");
  const [cooldown, setCooldown] = useState(60);
  const [pw, setPw] = useState("");
  const [confirm, setConfirm] = useState("");
  const [errors, setErrors] = useState<{ code?: string; pw?: string; confirm?: string }>({});
  const [formError, setFormError] = useState("");
  const [loading, setLoading] = useState(false);
  const [done, setDone] = useState(false);
  const strength = passwordStrength(pw);

  useEffect(() => {
    if (cooldown <= 0) return;
    const t = setTimeout(() => setCooldown((c) => c - 1), 1000);
    return () => clearTimeout(t);
  }, [cooldown]);

  if (!email) {
    return (
      <AuthLayout title="Reset your password" subtitle="Tell us your email first and we'll send you a 6-digit code.">
        <Button block onClick={() => navigate("/forgot-password")}>Send me a code</Button>
      </AuthLayout>
    );
  }
  if (done) {
    return (
      <AuthLayout title="Password updated" subtitle="You can now sign in with your new password.">
        <div className="form-ok" role="status"><Icon name="check" size={16} /> All set.</div>
        <Button block onClick={() => navigate("/login", { replace: true })}>Go to sign in</Button>
      </AuthLayout>
    );
  }

  const submit = async (ev: { preventDefault(): void }) => {
    ev.preventDefault();
    setFormError("");
    const digits = code.replace(/[\s-]/g, "");
    const e = { code: /^\d{6}$/.test(digits) ? "" : "Enter the 6-digit code from your email", pw: validatePassword(pw), confirm: validateConfirm(pw, confirm) };
    setErrors(e);
    if (e.code || e.pw || e.confirm) return;
    setLoading(true);
    try {
      await services.auth.resetPassword({ email, code: digits, password: pw });
      setDone(true);
    } catch (err) {
      setFormError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const resend = async () => {
    if (cooldown > 0) return;
    setCooldown(60);
    try {
      await services.auth.forgotPassword(email);
      toast.info("A new code is on its way.");
    } catch (err) {
      setFormError(errorMessage(err));
    }
  };
  const mockCode = config.mock && !config.liveAuth ? mockLastResetCode() : "";

  return (
    <AuthLayout title="Enter your code" subtitle={`If ${email} has an account, we sent a 6-digit code to it. It expires in 10 minutes.`}>
      <form className="auth-form" onSubmit={submit} noValidate>
        {formError && (
          <div className="form-error" role="alert">
            {formError}
          </div>
        )}
        {mockCode && <p className="field-hint">Mock mode (no email is sent): your code is <b>{mockCode}</b></p>}
        <Input label="6-digit code" icon="key" value={code} onChange={setCode} error={errors.code} inputMode="numeric" autoComplete="one-time-code" maxLength={7} autoFocus placeholder="123456" />
        <div>
          <Input label="New password" icon="lock" type="password" value={pw} onChange={setPw} error={errors.pw} autoComplete="new-password" />
          {pw && (
            <>
              <div className="strength" data-level={strength.level} aria-hidden="true"><i /><i /><i /><i /></div>
              <div className="field-hint" aria-live="polite">Password strength: {strength.label}</div>
            </>
          )}
        </div>
        <Input label="Confirm new password" icon="lock" type="password" value={confirm} onChange={setConfirm} error={errors.confirm} autoComplete="new-password" />
        <Button type="submit" block loading={loading}>Update password</Button>
      </form>
      <p className="auth-alt">
        {cooldown > 0 ? <span>Resend code in {cooldown}s</span> : <button type="button" className="link-btn" onClick={() => void resend()}>Resend code</button>}
        {" · "}
        <Link to="/forgot-password">Use a different email</Link>
      </p>
    </AuthLayout>
  );
}
