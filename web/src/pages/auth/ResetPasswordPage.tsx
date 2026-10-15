import { useState } from "react";
import { Button } from "@/components/common/Button";
import { Input } from "@/components/common/Input";
import { Icon } from "@/icons/Icon";
import { Link, navigate, useLocation } from "@/lib/router";
import { passwordStrength, validateConfirm, validatePassword } from "@/lib/validation";
import { errorMessage, services } from "@/services";
import { AuthLayout } from "./AuthLayout";

export default function ResetPasswordPage() {
  const { query } = useLocation();
  const token = query.get("token") ?? "";
  const [pw, setPw] = useState("");
  const [confirm, setConfirm] = useState("");
  const [errors, setErrors] = useState<{ pw?: string; confirm?: string }>({});
  const [formError, setFormError] = useState("");
  const [loading, setLoading] = useState(false);
  const [done, setDone] = useState(false);
  const strength = passwordStrength(pw);

  if (!token) {
    return (
      <AuthLayout title="Link not valid" subtitle="This password reset link is missing its token. Request a new one.">
        <Button block onClick={() => navigate("/forgot-password")}>Request a new link</Button>
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
    const e = { pw: validatePassword(pw), confirm: validateConfirm(pw, confirm) };
    setErrors(e);
    if (e.pw || e.confirm) return;
    setLoading(true);
    try {
      await services.auth.resetPassword({ token, password: pw });
      setDone(true);
    } catch (err) {
      setFormError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthLayout title="Set a new password" subtitle="Choose a strong password you don't use elsewhere.">
      <form className="auth-form" onSubmit={submit} noValidate>
        {formError && (
          <div className="form-error" role="alert">
            {formError} <Link to="/forgot-password">Request a new link</Link>
          </div>
        )}
        <div>
          <Input label="New password" icon="lock" type="password" value={pw} onChange={setPw} error={errors.pw} autoComplete="new-password" autoFocus />
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
    </AuthLayout>
  );
}
