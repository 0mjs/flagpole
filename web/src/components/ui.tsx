import { useEffect, useRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from "react";
import { Problem } from "../api/client";

export function Button({ variant = "default", className = "", ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "default" | "primary" | "danger" | "ghost" }) {
  const styles = {
    default: "bg-surface border-line-2 text-ink hover:bg-surface-2",
    primary: "bg-navy border-navy text-paper hover:opacity-90",
    danger: "bg-signal-red border-signal-red text-white hover:opacity-90",
    ghost: "bg-transparent border-transparent text-ink-2 hover:bg-surface-2",
  }[variant];
  return (
    <button
      {...props}
      className={`inline-flex items-center justify-center gap-1.5 rounded-md border px-3 py-1.5 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50 ${styles} ${className}`}
    />
  );
}

export function Field({ label, hint, error, children }: { label: string; hint?: string; error?: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-ink-2">{label}</span>
      {children}
      {error ? <span className="mt-1 block text-xs text-signal-red">{error}</span> : hint && <span className="mt-1 block text-xs text-muted">{hint}</span>}
    </label>
  );
}

const control = "w-full rounded-md border border-line-2 bg-surface px-3 py-2 text-sm text-ink placeholder:text-muted focus:border-signal-blue focus:outline-none";

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={`${control} ${props.className ?? ""}`} />;
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <span className={`relative inline-block ${props.className ?? "w-full"}`}>
      <select {...props} className={`${control} appearance-none pr-8`} />
      <span aria-hidden="true" className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-[10px] text-muted">▼</span>
    </span>
  );
}

/** A switch, styled as a hoist rope: on is a raised yellow toggle. */
export function Switch({ on, onChange, disabled, label }: { on: boolean; onChange: (v: boolean) => void; disabled?: boolean; label: string }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!on)}
      className={`relative h-6 w-11 shrink-0 rounded-full border transition disabled:cursor-not-allowed disabled:opacity-50 ${on ? "border-go bg-go" : "border-line-2 bg-surface-2"}`}
    >
      <span className={`absolute top-0.5 h-[18px] w-[18px] rounded-full bg-white shadow transition-all ${on ? "left-[22px]" : "left-0.5"}`} />
    </button>
  );
}

const roleTone: Record<string, string> = {
  admin: "bg-signal-red/12 text-signal-red border-signal-red/30",
  approver: "bg-signal-blue/12 text-signal-blue border-signal-blue/30",
  editor: "bg-signal-yellow/18 text-ink border-signal-yellow/50",
  viewer: "bg-surface-2 text-ink-2 border-line-2",
};

export function Badge({ children, tone }: { children: ReactNode; tone?: string }) {
  return (
    <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide ${roleTone[tone ?? ""] ?? "border-line-2 bg-surface-2 text-ink-2"}`}>
      {children}
    </span>
  );
}

/** The API's problem details: the message, and each field that's wrong. */
export function ProblemNote({ error }: { error: unknown }) {
  if (!error) return null;
  const p = error instanceof Problem ? error : null;
  return (
    <div role="alert" className="rounded-md border border-signal-red/40 bg-signal-red/8 px-3 py-2 text-sm text-ink">
      <strong className="text-signal-red">{p ? `${p.status} · ${p.detail ?? p.title}` : String(error)}</strong>
      {p && Object.keys(p.fields).length > 0 && (
        <ul className="mt-1 space-y-0.5">
          {Object.entries(p.fields).map(([k, v]) => (
            <li key={k} className="text-xs"><span className="mono">{k}</span> {v}</li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function Dialog({ open, onClose, title, children }: { open: boolean; onClose: () => void; title: string; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      className="m-auto w-[min(560px,92vw)] rounded-xl border border-line bg-surface p-0 text-ink shadow-2xl backdrop:bg-navy/40 backdrop:backdrop-blur-[2px]"
    >
      <div className="flex items-center justify-between border-b border-line px-5 py-3">
        <h2 className="font-display text-xl font-extrabold uppercase tracking-wide">{title}</h2>
        <button onClick={onClose} className="rounded p-1 text-muted hover:text-ink" aria-label="Close">✕</button>
      </div>
      <div className="p-5">{open && children}</div>
    </dialog>
  );
}

export function PageHeader({ eyebrow, title, children }: { eyebrow?: ReactNode; title: ReactNode; children?: ReactNode }) {
  return (
    <header className="mb-6 flex flex-wrap items-end justify-between gap-4">
      <div>
        {eyebrow && <div className="mb-1 text-xs font-semibold uppercase tracking-[0.18em] text-muted">{eyebrow}</div>}
        <h1 className="font-display text-4xl font-extrabold uppercase leading-none tracking-wide text-ink">{title}</h1>
      </div>
      <div className="flex items-center gap-2">{children}</div>
    </header>
  );
}

export function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="panel px-6 py-12 text-center">
      <div className="font-display text-2xl font-extrabold uppercase tracking-wide text-ink-2">{title}</div>
      {children && <div className="mx-auto mt-2 max-w-md text-sm text-muted">{children}</div>}
    </div>
  );
}

export function Loading() {
  return <div className="py-16 text-center text-sm text-muted">Hoisting…</div>;
}

export function relative(iso: string): string {
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "short" });
}
