import { useState } from "react";
import { Button } from "@/components/common/Button";
import { Input } from "@/components/common/Input";
import { Link, navigate } from "@/lib/router";
import { hasErrors, passwordStrength, validateConfirm, validateEmail, validateName, validatePassword, validatePhone, validateUsername } from "@/lib/validation";
import type { FieldErrors } from "@/lib/validation";
import { errorMessage, isServiceError, services } from "@/services";
import { config } from "@/services/config";
import { AuthLayout } from "./AuthLayout";

type Keys = "name" | "username" | "email" | "phone" | "password" | "confirm";

export default function RegisterPage() {
  const [v, setV] = useState<Record<Keys, string>>({ name: "", username: "", email: "", phone: "", password: "", confirm: "" });
  const [errors, setErrors] = useState<FieldErrors<Keys>>({});
  const [formError, setFormError] = useState("");
  const [loading, setLoading] = useState(false);
  const set = (k: Keys) => (val: string) => {
    setV((s) => ({ ...s, [k]: val }));
    if (errors[k]) setErrors((e) => ({ ...e, [k]: undefined }));
  };
  const check = (): FieldErrors<Keys> => ({
    name: validateName(v.name), username: validateUsername(v.username), email: validateEmail(v.email), phone: validatePhone(v.phone),
    password: validatePassword(v.password), confirm: validateConfirm(v.password, v.confirm),
  });
  const blur = (k: Keys) => () => v[k] && setErrors((e) => ({ ...e, [k]: check()[k] }));
  const strength = passwordStrength(v.password);

  const submit = async (ev: { preventDefault(): void }) => {
    ev.preventDefault();
    setFormError("");
    const e = check();
    setErrors(e);
    if (hasErrors(e)) return;
    setLoading(true);
    try {
      await services.auth.register({ name: v.name, username: v.username, email: v.email, phone: v.phone || undefined, password: v.password });
      navigate(config.liveAuth ? "/verify-email" : "/", { replace: true });
    } catch (err) {
      setFormError(errorMessage(err, "Couldn't create your account."));
      if (isServiceError(err) && err.fields) setErrors(err.fields as FieldErrors<Keys>);
      setLoading(false);
    }
  };

  return (
    <AuthLayout title="Create your account" subtitle="Small bites. Big conversations.">
      <form className="auth-form" onSubmit={submit} noValidate>
        {formError && <div className="form-error" role="alert">{formError}</div>}
        <Input label="Full name" icon="user" value={v.name} onChange={set("name")} onBlur={blur("name")} error={errors.name} autoComplete="name" autoFocus placeholder="Aisha Rahman" maxLength={50} />
        <Input label="Username" icon="at" value={v.username} onChange={(x) => set("username")(x.toLowerCase())} onBlur={blur("username")} error={errors.username} hint="Friends can find you by this username." autoComplete="username" placeholder="aisha" maxLength={24} />
        <Input label="Email" icon="mail" type="email" value={v.email} onChange={set("email")} onBlur={blur("email")} error={errors.email} autoComplete="email" inputMode="email" placeholder="name@example.com" />
        <Input label="Phone number (optional)" icon="phone" type="tel" value={v.phone} onChange={set("phone")} onBlur={blur("phone")} error={errors.phone} hint="Not verified — just so friends can recognise you." autoComplete="tel" inputMode="tel" placeholder="+8801XXXXXXXXX" />
        <div>
          <Input label="Password" icon="lock" type="password" value={v.password} onChange={set("password")} onBlur={blur("password")} error={errors.password} autoComplete="new-password" placeholder="At least 8 characters" />
          {v.password && (
            <>
              <div className="strength" data-level={strength.level} aria-hidden="true"><i /><i /><i /><i /></div>
              <div className="field-hint" aria-live="polite">Password strength: {strength.label}</div>
            </>
          )}
        </div>
        <Input label="Confirm password" icon="lock" type="password" value={v.confirm} onChange={set("confirm")} onBlur={blur("confirm")} error={errors.confirm} autoComplete="new-password" placeholder="Repeat your password" />
        <Button type="submit" block loading={loading}>Create account</Button>
      </form>
      <p className="auth-alt">Already have an account? <Link to="/login">Sign in</Link></p>
    </AuthLayout>
  );
}
