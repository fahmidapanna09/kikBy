import { useEffect, useRef, useState } from "react";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { EmptyState, ErrorState } from "@/components/common/EmptyState";
import { FriendButtons } from "@/components/common/FriendButtons";
import { Input } from "@/components/common/Input";
import { Modal } from "@/components/common/Modal";
import { Skeleton } from "@/components/common/Skeleton";
import { toast } from "@/components/common/Toast";
import { Icon } from "@/icons/Icon";
import { navigate } from "@/lib/router";
import { formatLastSeen, safeUrl } from "@/lib/utils";
import { hasErrors, validateName, validateUsername } from "@/lib/validation";
import type { FieldErrors } from "@/lib/validation";
import { services, useSession } from "@/services";
import { errorMessage, isServiceError } from "@/services/errors";
import { cacheUsers } from "@/state/actions";
import { friendActions } from "@/state/friends";
import { useApp } from "@/state/store";
import type { User } from "@/types/models";

type K = "displayName" | "username" | "bio";

function EditProfile({ me, onClose }: { me: User; onClose: () => void }) {
  const [v, setV] = useState({ displayName: me.displayName, username: me.username, bio: me.bio ?? "" });
  const [avatar, setAvatar] = useState(me.avatarUrl);
  const [errors, setErrors] = useState<FieldErrors<K | "avatar">>({});
  const [busy, setBusy] = useState(false);
  const file = useRef<HTMLInputElement | null>(null);
  const set = (k: K) => (val: string) => { setV((s) => ({ ...s, [k]: val })); setErrors((e) => ({ ...e, [k]: undefined })); };

  const pick = async (f?: File) => {
    if (!f) return;
    if (!f.type.startsWith("image/")) return setErrors((e) => ({ ...e, avatar: "Choose an image file" }));
    if (f.size > 5 * 1024 * 1024) return setErrors((e) => ({ ...e, avatar: "Image must be under 5 MB" }));
    try {
      const a = await services.uploads.upload(f);
      setAvatar(a.url);
      setErrors((e) => ({ ...e, avatar: undefined }));
    } catch (err) {
      setErrors((e) => ({ ...e, avatar: errorMessage(err) }));
    }
  };

  const save = async (ev: { preventDefault(): void }) => {
    ev.preventDefault();
    const e: FieldErrors<K> = { displayName: validateName(v.displayName), username: validateUsername(v.username), bio: v.bio.length > 160 ? "Bio is limited to 160 characters" : "" };
    setErrors(e);
    if (hasErrors(e)) return;
    setBusy(true);
    try {
      const u = await services.users.updateProfile({ displayName: v.displayName, username: v.username, bio: v.bio, avatarUrl: avatar });
      cacheUsers([u]);
      toast.success("Profile updated");
      onClose();
    } catch (err) {
      if (isServiceError(err) && err.fields) setErrors(err.fields as FieldErrors<K>);
      else toast.error(errorMessage(err));
      setBusy(false);
    }
  };

  return (
    <Modal title="Edit profile" onClose={onClose} footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button type="submit" loading={busy} onClick={save}>Save</Button></>}>
      <form className="auth-form" onSubmit={save} noValidate>
        <div className="avatar-edit">
          <Avatar name={v.displayName || me.displayName} id={me.id} src={avatar} size={72} />
          <input ref={file} type="file" accept="image/*" hidden tabIndex={-1} aria-hidden="true" onChange={(e: any) => void pick(e.target.files?.[0])} />
          <Button size="sm" variant="secondary" onClick={() => file.current?.click()}><Icon name="camera" size={16} /> Change photo</Button>
          {avatar && <Button size="sm" variant="ghost" onClick={() => setAvatar(undefined)}>Remove</Button>}
        </div>
        {errors.avatar && <div className="field-error" role="alert">{errors.avatar}</div>}
        <Input label="Display name" value={v.displayName} onChange={set("displayName")} error={errors.displayName} maxLength={50} autoComplete="name" />
        <Input label="Username" icon="at" value={v.username} onChange={(x) => set("username")(x.toLowerCase())} error={errors.username} maxLength={24} autoComplete="username" />
        <div className="field">
          <label className="field-label" htmlFor="bio">Bio</label>
          <div className={`input-wrap${errors.bio ? " invalid" : ""}`}>
            <textarea id="bio" className="input" rows={3} value={v.bio} onChange={(e: any) => set("bio")(e.target.value)} maxLength={200} style={{ resize: "none" }} />
          </div>
          <div className={errors.bio ? "field-error" : "field-hint"}>{errors.bio ?? `${v.bio.length}/160`}</div>
        </div>
        <Input label="Email" value={me.email ?? ""} onChange={() => {}} disabled hint="Change your email in Settings → Account." />
      </form>
    </Modal>
  );
}

export default function ProfilePage({ id }: { id?: string }) {
  const sess = useSession();
  const meId = sess?.user.id;
  const targetId = id ?? meId ?? "";
  const isMe = targetId === meId;
  const cached = useApp((s) => s.users[targetId]);
  const [user, setUser] = useState<User | null>(null);
  const [state, setState] = useState<"loading" | "ready" | "error" | "missing">("loading");
  const [editing, setEditing] = useState(false);
  const friend = useApp((s) => s.friends.find((f) => f.user.id === targetId));

  const load = () => {
    setState("loading");
    services.users.getProfile(targetId).then((u) => { setUser(u); cacheUsers([u]); setState("ready"); }).catch((e) => setState(isServiceError(e) && e.status === 404 ? "missing" : "error"));
  };
  useEffect(load, [targetId]); // eslint-disable-line react-hooks/exhaustive-deps

  const u = (isMe ? cached ?? sess?.user : cached ?? user) as User | null;

  if (state === "loading" && !u) return <div className="page"><div className="page-inner"><Skeleton h={120} /><Skeleton h={20} w="50%" /><Skeleton h={80} /></div></div>;
  if (state === "missing") return <div className="page" style={{ display: "flex" }}><EmptyState icon="user" title="Profile not found" message="This user doesn't exist." actions={<Button size="sm" onClick={() => navigate("/")}>Go home</Button>} /></div>;
  if (state === "error" && !u) return <div className="page" style={{ display: "flex" }}><ErrorState message="Couldn't load this profile." onRetry={load} /></div>;
  if (!u) return null;

  const blocked = friend?.status === "blocked";
  return (
    <div className="page">
      <div className="page-inner">
        <section className="card profile-card">
          <div className="profile-cover" aria-hidden="true" />
          <div className="profile-body">
            <Avatar name={u.displayName} id={u.id} src={safeUrl(u.avatarUrl)} size={96} presence={isMe ? "online" : u.presence} />
            <h2>{u.displayName}</h2>
            <p className="profile-handle">@{u.username}{!isMe && ` · ${formatLastSeen(u)}`}</p>
            {u.bio ? <p className="profile-bio">{u.bio}</p> : isMe ? <p className="field-hint">Add a bio so friends know who you are.</p> : null}
            <div className="profile-actions">
              {isMe ? (
                <>
                  <Button onClick={() => setEditing(true)}><Icon name="edit" size={16} /> Edit profile</Button>
                  <Button variant="secondary" onClick={() => navigate("/settings")}><Icon name="settings" size={16} /> Settings</Button>
                </>
              ) : (
                <>
                  <FriendButtons user={u} />
                  {!blocked && friend?.status !== "blocked" && <Button size="sm" variant="ghost" onClick={() => void friendActions.block(u)}><Icon name="block" size={16} /> Block</Button>}
                </>
              )}
            </div>
          </div>
        </section>
        {!isMe && friend?.mutualCount ? <p className="field-hint">{friend.mutualCount} mutual friends</p> : null}
      </div>
      {editing && sess && <EditProfile me={u} onClose={() => setEditing(false)} />}
    </div>
  );
}
