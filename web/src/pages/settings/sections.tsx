import { useState } from "react";
import { Button } from "@/components/common/Button";
import { Input } from "@/components/common/Input";
import { Modal } from "@/components/common/Modal";
import { toast } from "@/components/common/Toast";
import { Link, navigate } from "@/lib/router";
import { hasErrors, passwordStrength, validateConfirm, validateEmail, validatePassword, validatePhone } from "@/lib/validation";
import { services, useSession } from "@/services";
import { config } from "@/services/config";
import { errorMessage, isServiceError } from "@/services/errors";
import { cacheUsers } from "@/state/actions";
import { Row, Section, SelectRow, ToggleRow, useSettings } from "./common";

export function AccountSection() {
  const me = useSession()?.user;
  const [email, setEmail] = useState(me?.email ?? "");
  const [emailPw, setEmailPw] = useState("");
  const [emailPwErr, setEmailPwErr] = useState("");
  const [emailNote, setEmailNote] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [phone, setPhone] = useState(me?.phone ?? "");
  const [perr, setPerr] = useState("");
  const [pbusy, setPbusy] = useState(false);
  if (!me) return null;
  const savePhone = async (ev: { preventDefault(): void }) => {
    ev.preventDefault();
    const e = validatePhone(phone);
    setPerr(e);
    if (e) return;
    setPbusy(true);
    try {
      cacheUsers([await services.users.updateProfile({ phone })]);
      toast.success(phone.trim() ? "Phone number saved" : "Phone number removed");
    } catch (x) {
      setPerr(isServiceError(x) && x.fields?.phone ? x.fields.phone : errorMessage(x));
    } finally {
      setPbusy(false);
    }
  };
  const save = async (ev: { preventDefault(): void }) => {
    ev.preventDefault();
    const e = validateEmail(email);
    const pe = config.liveAuth && !emailPw ? "Enter your current password" : "";
    setErr(e);
    setEmailPwErr(pe);
    if (e || pe) return;
    setBusy(true);
    try {
      const r = await services.users.changeEmail({ email, password: emailPw });
      cacheUsers([r.user]);
      setEmailPw("");
      if (r.pending) {
        setEmail(r.user.email ?? "");
        setEmailNote(`We sent a 6-digit code to ${r.user.pendingEmail ?? email.trim()}. Your sign-in email changes once you enter it (valid for 10 minutes).`);
      } else toast.success("Email updated");
    } catch (x) {
      if (isServiceError(x) && x.fields?.password) setEmailPwErr("Incorrect password");
      else setErr(isServiceError(x) && x.fields?.email ? x.fields.email : errorMessage(x));
    } finally {
      setBusy(false);
    }
  };
  const resendPending = async () => {
    try {
      await services.auth.resendVerification();
      toast.success("New code sent");
    } catch (x) {
      toast.error(errorMessage(x));
    }
  };
  return (
    <>
      <Section title="Profile">
        <Row title={me.displayName} desc={`@${me.username}`}><Link to="/profile" className="btn btn-secondary btn-sm">Edit profile</Link></Row>
      </Section>
      <Section title="Email address" desc="Used to sign in and to reset your password.">
        <form className="set-form" onSubmit={save} noValidate>
          <Input label="Email" type="email" icon="mail" value={email} onChange={(v) => { setEmail(v); setErr(""); setEmailNote(""); }} error={err} autoComplete="email" />
          {config.liveAuth && email.trim().toLowerCase() !== (me.email ?? "").toLowerCase() && (
            <Input label="Current password" type="password" icon="lock" value={emailPw} onChange={(v) => { setEmailPw(v); setEmailPwErr(""); }} error={emailPwErr} autoComplete="current-password" />
          )}
          {(me.pendingEmail || emailNote) && (
            <div className="field-hint" role="status">
              {emailNote || `Waiting for you to confirm ${me.pendingEmail}.`} <Link to="/verify-email" className="banner-action">Enter code</Link> <button type="button" className="banner-action" style={{ marginLeft: 0 }} onClick={resendPending}>Resend code</button>
            </div>
          )}
          <div><Button type="submit" size="sm" loading={busy} disabled={email.trim().toLowerCase() === (me.email ?? "").toLowerCase()}>{config.liveAuth ? "Change email" : "Save email"}</Button></div>
        </form>
      </Section>
      <Section title="Phone number" desc="Optional. Not verified — no SMS code is sent.">
        <form className="set-form" onSubmit={savePhone} noValidate>
          <Input label="Phone number" type="tel" icon="phone" value={phone} onChange={(v) => { setPhone(v); setPerr(""); }} error={perr} autoComplete="tel" inputMode="tel" placeholder="+8801XXXXXXXXX" />
          <div><Button type="submit" size="sm" loading={pbusy} disabled={phone.trim() === (me.phone ?? "")}>Save phone number</Button></div>
        </form>
      </Section>
    </>
  );
}

export function PrivacySection() {
  const { settings: s, update } = useSettings();
  if (!s) return null;
  const who: Array<["everyone" | "friends" | "nobody", string]> = [["everyone", "Everyone"], ["friends", "Friends only"], ["nobody", "Nobody"]];
  return (
    <>
      <Section title="Visibility">
        <SelectRow title="Last seen & online" desc="Who can see when you were last active." value={s.privacy.lastSeen} options={who} onChange={(v) => void update("privacy", { lastSeen: v })} />
        <SelectRow title="Profile photo" desc="Who can see your profile photo." value={s.privacy.profilePhoto} options={who} onChange={(v) => void update("privacy", { profilePhoto: v })} />
      </Section>
      <Section title="Messaging">
        <ToggleRow title="Read receipts" desc="If off, you won't send or see read receipts." checked={s.privacy.readReceipts} onChange={(v) => void update("privacy", { readReceipts: v })} />
        <ToggleRow title="Typing indicator" desc="Let others see when you're typing." checked={s.privacy.typingIndicator} onChange={(v) => void update("privacy", { typingIndicator: v })} />
      </Section>
      <Section title="Blocked users">
        <Row title="Manage blocked users" desc="People you block can't message you."><Button size="sm" variant="secondary" onClick={() => navigate("/friends")}>Open</Button></Row>
      </Section>
    </>
  );
}

export function NotificationsSection() {
  const { settings: s, update } = useSettings();
  if (!s) return null;
  const n = s.notifications;
  return (
    <>
      <Section title="Alerts">
        <ToggleRow title="Messages" desc="New direct messages." checked={n.messages} onChange={(v) => void update("notifications", { messages: v })} />
        <ToggleRow title="Groups" desc="Messages in group chats." checked={n.groups} onChange={(v) => void update("notifications", { groups: v })} />
        <ToggleRow title="Friend requests" checked={n.friendRequests} onChange={(v) => void update("notifications", { friendRequests: v })} />
      </Section>
      <Section title="Behaviour">
        <ToggleRow title="Sound" checked={n.sound} onChange={(v) => void update("notifications", { sound: v })} />
        <ToggleRow title="Show message preview" desc="Show message text inside notifications." checked={n.preview} onChange={(v) => void update("notifications", { preview: v })} />
      </Section>
    </>
  );
}

export function AppearanceSection() {
  const { settings: s, update } = useSettings();
  if (!s) return null;
  const a = s.appearance;
  return (
    <>
      <Section title="Theme">
        <Row title="Dark" desc="KikBite is dark-first. A light theme isn't part of the current design."><span className="badge muted">Active</span></Row>
      </Section>
      <Section title="Display">
        <SelectRow title="Density" desc="Spacing of lists." value={a.density} options={[["comfortable", "Comfortable"], ["compact", "Compact"]]} onChange={(v) => void update("appearance", { density: v })} />
        <SelectRow title="Text size" value={a.fontScale} options={[[90, "Small"], [100, "Default"], [110, "Large"], [120, "Extra large"]]} onChange={(v) => void update("appearance", { fontScale: v })} />
        <ToggleRow title="Reduce motion" desc="Minimise animations. Also follows your system setting." checked={a.reduceMotion} onChange={(v) => void update("appearance", { reduceMotion: v })} />
      </Section>
    </>
  );
}

export function ChatSection() {
  const { settings: s, update } = useSettings();
  if (!s) return null;
  const c = s.chat;
  return (
    <Section title="Messages">
      <ToggleRow title="Enter sends message" desc="Off: Enter adds a new line, Ctrl/⌘+Enter sends." checked={c.enterToSend} onChange={(v) => void update("chat", { enterToSend: v })} />
      <ToggleRow title="Auto-download media" desc="Load images automatically." checked={c.mediaAutoDownload} onChange={(v) => void update("chat", { mediaAutoDownload: v })} />
      <ToggleRow title="Spell check" checked={c.spellcheck} onChange={(v) => void update("chat", { spellcheck: v })} />
    </Section>
  );
}

export function SecuritySection() {
  const [v, setV] = useState({ current: "", next: "", confirm: "" });
  const [errors, setErrors] = useState<{ current?: string; next?: string; confirm?: string }>({});
  const [busy, setBusy] = useState(false);
  const st = passwordStrength(v.next);
  const submit = async (ev: { preventDefault(): void }) => {
    ev.preventDefault();
    const e = { current: v.current ? "" : "Enter your current password", next: validatePassword(v.next) || (v.next === v.current ? "Choose a different password" : ""), confirm: validateConfirm(v.next, v.confirm) };
    setErrors(e);
    if (hasErrors(e)) return;
    setBusy(true);
    try {
      await services.users.changePassword({ current: v.current, next: v.next });
      toast.success("Password changed");
      setV({ current: "", next: "", confirm: "" });
    } catch (x) {
      if (isServiceError(x) && x.fields?.current) setErrors({ current: x.fields.current });
      else toast.error(errorMessage(x));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <Section title="Change password">
        <form className="set-form" onSubmit={submit} noValidate>
          <Input label="Current password" type="password" icon="lock" value={v.current} onChange={(x) => setV((s) => ({ ...s, current: x }))} error={errors.current} autoComplete="current-password" />
          <div>
            <Input label="New password" type="password" icon="lock" value={v.next} onChange={(x) => setV((s) => ({ ...s, next: x }))} error={errors.next} autoComplete="new-password" />
            {v.next && <><div className="strength" data-level={st.level} aria-hidden="true"><i /><i /><i /><i /></div><div className="field-hint" aria-live="polite">Password strength: {st.label}</div></>}
          </div>
          <Input label="Confirm new password" type="password" icon="lock" value={v.confirm} onChange={(x) => setV((s) => ({ ...s, confirm: x }))} error={errors.confirm} autoComplete="new-password" />
          <div><Button type="submit" size="sm" loading={busy}>Update password</Button></div>
        </form>
      </Section>
      <Section title="Sessions">
        <Row title="This device" desc="Signed in now"><span className="badge muted">Current</span></Row>
        <Row title="Sign out everywhere" desc="Ends your session on this device."><Button size="sm" variant="danger" onClick={() => void services.auth.logout()}>Sign out</Button></Row>
      </Section>
    </>
  );
}

export function OtherSection() {
  const me = useSession()?.user;
  const [del, setDel] = useState(false);
  const [pw, setPw] = useState("");
  const [typed, setTyped] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const remove = async () => {
    setBusy(true);
    try {
      await services.users.deleteAccount({ password: pw });
      await services.auth.logout();
      navigate("/login", { replace: true });
    } catch (x) {
      setErr(isServiceError(x) && x.fields?.password ? x.fields.password : errorMessage(x));
      setBusy(false);
    }
  };
  return (
    <>
      <Section title="About">
        <Row title="KikBite" desc="Small bites. Big conversations."><span className="set-desc">v0.1.0</span></Row>
        <Row title="Help & feedback" desc="support@kikbite.com"><a className="btn btn-secondary btn-sm" href="mailto:support@kikbite.com">Contact</a></Row>
      </Section>
      <Section title="Account actions">
        <Row title="Sign out"><Button size="sm" variant="secondary" onClick={() => void services.auth.logout()}>Sign out</Button></Row>
        <Row title="Delete account" desc="Permanently deletes your account. This can't be undone."><Button size="sm" variant="danger" onClick={() => setDel(true)}>Delete…</Button></Row>
      </Section>
      {del && me && (
        <Modal title="Delete account?" onClose={() => setDel(false)} footer={<><Button variant="ghost" onClick={() => setDel(false)}>Cancel</Button><Button variant="danger" loading={busy} disabled={typed !== me.username || !pw} onClick={() => void remove()}>Delete forever</Button></>}>
          <div className="set-form">
            <p>This permanently removes your account and sign-in. Type <b>{me.username}</b> and enter your password to confirm.</p>
            <Input label="Type your username" value={typed} onChange={setTyped} data-autofocus autoComplete="off" />
            <Input label="Password" type="password" value={pw} onChange={(x) => { setPw(x); setErr(""); }} error={err} autoComplete="current-password" />
          </div>
        </Modal>
      )}
    </>
  );
}
