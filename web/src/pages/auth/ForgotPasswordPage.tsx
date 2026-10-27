import { useState } from "react";
import { Button } from "@/components/common/Button";
import { Input } from "@/components/common/Input";
import { Link, navigate } from "@/lib/router";
import { validateEmail } from "@/lib/validation";
import { errorMessage, services } from "@/services";
import { AuthLayout } from "./AuthLayout";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [formError, setFormError] = useState("");
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
      // Same answer whether or not the address has an account: go straight to the code screen.
      navigate(`/reset-password?email=${encodeURIComponent(email.trim())}`);
    } catch (err) {
      setFormError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthLayout title="Forgot password?" subtitle="Enter your email and we'll send you a 6-digit code.">
      <form className="auth-form" onSubmit={submit} noValidate>
        {formError && <div className="form-error" role="alert">{formError}</div>}
        <Input label="Email" icon="mail" type="email" value={email} onChange={setEmail} error={error} autoComplete="email" inputMode="email" autoFocus placeholder="name@example.com" />
        <Button type="submit" block loading={loading}>Send code</Button>
      </form>
      <p className="auth-alt"><Link to="/login">Back to sign in</Link></p>
    </AuthLayout>
  );
}
