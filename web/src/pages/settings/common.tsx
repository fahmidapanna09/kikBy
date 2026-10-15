import { useId } from "react";
import type { ReactNode } from "react";
import { Switch } from "@/components/common/Input";
import { toast } from "@/components/common/Toast";
import { services } from "@/services";
import { errorMessage } from "@/services/errors";
import { store, useApp } from "@/state/store";
import type { Settings } from "@/types/models";

/** Optimistic settings update with rollback. Settings apply instantly (appearance is re-applied by AppShell). */
export function useSettings() {
  const settings = useApp((s) => s.settings);
  const update = async <K extends keyof Settings>(section: K, patch: Partial<Settings[K]>) => {
    const prev = store.get().settings;
    if (!prev) return;
    const next = { ...prev, [section]: { ...prev[section], ...patch } };
    store.set({ settings: next });
    try {
      await services.users.updateSettings(next);
    } catch (e) {
      store.set({ settings: prev });
      toast.error(errorMessage(e, "Couldn't save that setting."));
    }
  };
  return { settings, update };
}

export function Row({ title, desc, children, htmlFor }: { title: string; desc?: string; children: ReactNode; htmlFor?: string }) {
  return (
    <div className="set-row">
      <div className="set-text">
        <label htmlFor={htmlFor} className="set-title">{title}</label>
        {desc && <p className="set-desc">{desc}</p>}
      </div>
      <div className="set-control">{children}</div>
    </div>
  );
}

export function ToggleRow({ title, desc, checked, onChange }: { title: string; desc?: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="set-row">
      <div className="set-text"><span className="set-title">{title}</span>{desc && <p className="set-desc">{desc}</p>}</div>
      <div className="set-control"><Switch checked={checked} onChange={onChange} label={title} /></div>
    </div>
  );
}

export function SelectRow<T extends string | number>({ title, desc, value, options, onChange }: { title: string; desc?: string; value: T; options: Array<[T, string]>; onChange: (v: T) => void }) {
  const id = useId();
  return (
    <Row title={title} desc={desc} htmlFor={id}>
      <select id={id} className="select" value={String(value)} onChange={(e: any) => onChange((typeof value === "number" ? Number(e.target.value) : e.target.value) as T)}>
        {options.map(([v, l]) => <option key={String(v)} value={String(v)}>{l}</option>)}
      </select>
    </Row>
  );
}

export function Section({ title, desc, children }: { title: string; desc?: string; children: ReactNode }) {
  return (
    <section className="set-section" aria-label={title}>
      <h3>{title}</h3>
      {desc && <p className="set-desc" style={{ marginBottom: 8 }}>{desc}</p>}
      <div className="card set-card">{children}</div>
    </section>
  );
}
