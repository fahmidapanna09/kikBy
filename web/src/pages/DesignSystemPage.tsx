import { useState } from "react";
import { Avatar } from "@/components/common/Avatar";
import { Button, IconButton } from "@/components/common/Button";
import { Checkbox, Input, Switch } from "@/components/common/Input";
import { ConfirmDialog, Drawer, Modal } from "@/components/common/Modal";
import { Dropdown } from "@/components/common/Dropdown";
import { EmptyState, ErrorState } from "@/components/common/EmptyState";
import { ListSkeleton } from "@/components/common/Skeleton";
import { toast } from "@/components/common/Toast";
import { Icon } from "@/icons/Icon";
import type { IconName } from "@/icons/Icon";
import { Logo } from "@/icons/Logo";

const ICONS: IconName[] = ["back", "search", "video", "phone", "plus", "send", "mic", "camera", "attach", "image", "doc", "check", "check-double", "close", "settings", "chat", "users", "user", "user-plus", "home", "bell", "lock", "mail", "at", "eye", "reply", "forward", "edit", "trash", "copy", "smile", "star", "pin", "block", "logout", "alert", "info", "clock", "shield", "palette", "globe", "menu", "download", "wifi-off", "refresh"];

/** Dev-only page at /_ds: every shared component in one place for visual QA. */
export default function DesignSystemPage() {
  const [modal, setModal] = useState<"none" | "modal" | "confirm" | "drawer">("none");
  const [text, setText] = useState("");
  const [on, setOn] = useState(true);
  const [chk, setChk] = useState(false);
  return (
    <div className="ds-page">
      <Logo size={44} tagline />
      <section className="ds-section">
        <h2>Buttons</h2>
        <div className="ds-row">
          <Button>Primary</Button>
          <Button variant="secondary">Secondary</Button>
          <Button variant="ghost">Ghost</Button>
          <Button variant="danger">Danger</Button>
          <Button loading>Loading</Button>
          <Button disabled>Disabled</Button>
          <IconButton label="Search"><Icon name="search" /></IconButton>
        </div>
      </section>
      <section className="ds-section">
        <h2>Inputs</h2>
        <div style={{ maxWidth: 360, display: "grid", gap: 14 }}>
          <Input label="Email or username" icon="user" value={text} onChange={setText} placeholder="name@example.com" />
          <Input label="Password" type="password" icon="lock" value="secret123" onChange={() => {}} />
          <Input label="With error" value="bad" onChange={() => {}} error="Enter a valid email address" />
          <Checkbox checked={chk} onChange={setChk}>Remember me</Checkbox>
          <label style={{ display: "flex", gap: 12, alignItems: "center" }}><Switch checked={on} onChange={setOn} label="Notifications" /> Notifications</label>
        </div>
      </section>
      <section className="ds-section">
        <h2>Avatars & badges</h2>
        <div className="ds-row">
          {["Aisha Rahman", "Rafi Khan", "Nabila", "Tanvir", "Mim", "Sakib"].map((n, i) => <Avatar key={n} name={n} tone={((i % 6) + 1) as 1} presence={i % 3 === 0 ? "online" : i % 3 === 1 ? "away" : "offline"} size={48} />)}
          <Avatar name="KikBite Team" group size={48} tone={3} />
          <span className="badge">3</span><span className="badge muted">12</span>
        </div>
      </section>
      <section className="ds-section">
        <h2>Overlays</h2>
        <div className="ds-row">
          <Button variant="secondary" onClick={() => setModal("modal")}>Modal</Button>
          <Button variant="secondary" onClick={() => setModal("confirm")}>Confirm</Button>
          <Button variant="secondary" onClick={() => setModal("drawer")}>Drawer</Button>
          <Dropdown label="More options" items={[{ label: "Reply", icon: "reply", onSelect: () => toast.info("Reply") }, { label: "Copy", icon: "copy", onSelect: () => toast.success("Copied") }, { label: "Delete", icon: "trash", danger: true, separatorBefore: true, onSelect: () => toast.error("Deleted", { actionLabel: "Undo", onAction: () => toast.info("Restored") }) }]}><Icon name="more" /></Dropdown>
        </div>
      </section>
      <section className="ds-section">
        <h2>States</h2>
        <div style={{ display: "grid", gap: 16, gridTemplateColumns: "repeat(auto-fit,minmax(280px,1fr))" }}>
          <div style={{ background: "var(--surface)", borderRadius: 16 }}><ListSkeleton rows={3} /></div>
          <div style={{ background: "var(--surface)", borderRadius: 16, display: "flex" }}><EmptyState icon="chat" title="No conversations yet" message="Start a chat with a friend." actions={<Button size="sm">New chat</Button>} /></div>
          <div style={{ background: "var(--surface)", borderRadius: 16, display: "flex" }}><ErrorState onRetry={() => toast.info("Retrying…")} /></div>
        </div>
        <div className="banner warning"><Icon name="wifi-off" size={16} /> You're offline — messages will send when you reconnect.</div>
      </section>
      <section className="ds-section">
        <h2>Icons</h2>
        <div className="ds-row" style={{ color: "var(--text-dim)" }}>{ICONS.map((n) => <span key={n} title={n}><Icon name={n} size={24} /></span>)}</div>
      </section>
      {modal === "modal" && <Modal title="Modal title" onClose={() => setModal("none")} footer={<Button onClick={() => setModal("none")}>Done</Button>}>Focus is trapped here. Press Esc to close.</Modal>}
      {modal === "confirm" && <ConfirmDialog title="Delete message?" message="This can't be undone." danger confirmLabel="Delete" onConfirm={() => toast.success("Deleted")} onClose={() => setModal("none")} />}
      {modal === "drawer" && <Drawer title="Drawer" onClose={() => setModal("none")}>Bottom sheet on mobile, side drawer on desktop.</Drawer>}
    </div>
  );
}
