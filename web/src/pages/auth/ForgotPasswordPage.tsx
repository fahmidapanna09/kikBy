import { useState } from "react";
import { Button } from "@/components/common/Button";
import { Input } from "@/components/common/Input";
import { Icon } from "@/icons/Icon";
import { Link } from "@/lib/router";
import { validateEmail } from "@/lib/validation";
import { config } from "@/services/config";
import { errorMessage, mockLastResetLink, services } from "@/services";
import { AuthLayout } from "./AuthLayout";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [formError, setFormError] = useState("");
  const [sent, setSent] = useState(false);
  const [loading, setLoading] = useState(false);

  const submit = async (ev: { preventDefault(): void }) => {
    ev.preventDefault();
    setFormError("");
    const e = validateEmail(email);
    setError(e);
    if (e) return;
    setLoading(true);
    try {
      await services.auth.forgotPassword(email);
      setSent(true);
    } catch (err) {
      setFormError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  if (sent) {
    const link = config.mock && !config.liveAuth ? mockLastResetLink() : "";
    return (
      <AuthLayout title="Check your email" subtitle={`If an account exists for ${email.trim()}, we've sent a link to reset your password.`}>
        <div className="form-ok" role="status"><Icon name="mail" size={16} /> The link expires in 30 minutes.</div>
        {link && <p className="field-hint">Mock mode (no email is sent): <Link to={link}>open reset link</Link></p>}
        <Button variant="secondary" block onClick={() => setSent(false)}>Use a different email</Button>
        <p className="auth-alt"><Link to="/login">Back to sign in</Link></p>
      </AuthLayout>
    );
  }
  return (
    <AuthLayout title="Forgot password?" subtitle="Enter your email and we'll send you a reset link.">
      <form className="auth-form" onSubmit={submit} noValidate>
        {formError && <div className="form-error" role="alert">{formError}</div>}
        <Input label="Email" icon="mail" type="email" value={email} onChange={setEmail} error={error} autoComplete="email" inputMode="email" autoFocus placeholder="name@example.com" />
        <Button type="submit" block loading={loading}>Send reset link</Button>
      </form>
      <p className="auth-alt"><Link to="/login">Back to sign in</Link></p>
    </AuthLayout>
  );
}
