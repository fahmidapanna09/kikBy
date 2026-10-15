import { useState } from "react";
import { Button } from "@/components/common/Button";
import { Checkbox, Input } from "@/components/common/Input";
import { Link, navigate, useLocation } from "@/lib/router";
import { validateIdentifier, required } from "@/lib/validation";
import { errorMessage, isServiceError, services } from "@/services";
import { AuthLayout, safeNext } from "./AuthLayout";

export default function LoginPage() {
  const { query } = useLocation();
  const [identifier, setIdentifier] = useState("");
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(true);
  const [errors, setErrors] = useState<{ identifier?: string; password?: string }>({});
  const [formError, setFormError] = useState("");
  const [loading, setLoading] = useState(false);
  const expired = query.get("expired") === "1";

  const validate = () => {
    const e = { identifier: validateIdentifier(identifier), password: required(password, "Password") };
    setErrors(e);
    return !e.identifier && !e.password;
  };

  const submit = async (ev: { preventDefault(): void }) => {
    ev.preventDefault();
    setFormError("");
    if (!validate()) return;
    setLoading(true);
    try {
      await services.auth.login({ identifier, password, remember });
      navigate(safeNext(query.get("next")), { replace: true });
    } catch (e) {
      setFormError(errorMessage(e, "Couldn't sign in. Please try again."));
      if (isServiceError(e) && e.fields) setErrors(e.fields);
      setLoading(false);
    }
  };

  return (
    <AuthLayout title="Welcome back" subtitle="Sign in to continue your conversations.">
      <form className="auth-form" onSubmit={submit} noValidate>
        {expired && !formError && <div className="form-error" role="alert">Your session expired. Please sign in again.</div>}
        {formError && <div className="form-error" role="alert">{formError}</div>}
        <Input label="Email or username" icon="user" value={identifier} onChange={setIdentifier} onBlur={() => identifier && setErrors((e) => ({ ...e, identifier: validateIdentifier(identifier) }))} error={errors.identifier} autoComplete="username" autoFocus placeholder="name@example.com" />
        <Input label="Password" icon="lock" type="password" value={password} onChange={setPassword} error={errors.password} autoComplete="current-password" placeholder="Your password" />
        <div className="auth-row">
          <Checkbox checked={remember} onChange={setRemember}>Remember me</Checkbox>
          <Link to="/forgot-password">Forgot password?</Link>
        </div>
        <Button type="submit" block loading={loading}>Sign in</Button>
      </form>
      <p className="auth-alt">New to KikBite? <Link to="/register">Create an account</Link></p>
    </AuthLayout>
  );
}
