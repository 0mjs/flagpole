import { useEffect, useState } from "react";
import type { Result } from "@flagpole/sdk";
import { useAllFlags, useFlagpole, useFlagpoleStatus } from "@flagpole/sdk/react";
import { people } from "./people";

const dashboard = (import.meta.env.VITE_FLAGPOLE_DASHBOARD ?? "http://localhost:5173/projects/web-app").replace(/\/$/, "");

function why(r: Result): string {
  const n = (r.rule ?? 0) + 1;
  switch (r.reason) {
    case "off":
      return "flag is off";
    case "rule":
      return `rule ${n} matched`;
    case "rollout":
      return `rule ${n}, by percentage`;
    case "default":
      return "on, no rule matched";
    default:
      return "not found";
  }
}

interface Logged {
  at: Date;
  flags: { key: string; variant: string }[];
}

/** The developer's view of the shop: who you are, every flag, and changes as they land. */
export function Inspector({ person, onPerson, onNewVisitor, xray, onXray }: { person: string; onPerson: (id: string) => void; onNewVisitor: () => void; xray: boolean; onXray: (v: boolean) => void }) {
  const client = useFlagpole();
  const flags = useAllFlags();
  const status = useFlagpoleStatus();
  const [open, setOpen] = useState(() => window.innerWidth >= 900);
  const [log, setLog] = useState<Logged[]>([]);

  useEffect(
    () =>
      client.on("change", ({ flags: keys }) => {
        const all = client.all();
        setLog((l) => [{ at: new Date(), flags: keys.map((key) => ({ key, variant: all[key]?.variant ?? "removed" })) }, ...l].slice(0, 4));
      }),
    [client],
  );

  const dot = { live: "bg-emerald-400", connecting: "bg-amber-300", reconnecting: "bg-amber-300 animate-pulse", closed: "bg-red-400" }[status];
  const ctx = client.context;

  if (!open) {
    return (
      <button onClick={() => setOpen(true)} className="fixed bottom-4 left-4 z-40 flex items-center gap-2 rounded-full bg-ink px-4 py-2 font-mono text-xs text-white shadow-xl">
        <span className={`h-2 w-2 rounded-full ${dot}`} /> Flagpole · {Object.keys(flags).length} flags
      </button>
    );
  }

  return (
    <aside className="slide-in fixed bottom-4 left-4 z-40 flex max-h-[calc(100vh-2rem)] w-[min(360px,calc(100vw-2rem))] flex-col overflow-hidden rounded-2xl bg-ink text-[#e8e4da] shadow-2xl ring-1 ring-black/20">
      <header className="flex items-center justify-between border-b border-white/10 px-4 py-3">
        <div className="flex items-center gap-2">
          <svg viewBox="0 0 32 32" className="h-5 w-5" aria-hidden="true">
            <rect x="5" y="2" width="2.5" height="28" rx="1" fill="#e8e4da" />
            <path d="M7.5 4.5h18v11h-18z" fill="#f2b705" />
            <path d="M7.5 4.5h18v11z" fill="#c8102e" />
          </svg>
          <span className="font-mono text-xs font-medium tracking-wide">FLAGPOLE SDK</span>
        </div>
        <div className="flex items-center gap-3">
          <span className="flex items-center gap-1.5 font-mono text-[11px] text-white/60">
            <span className={`h-2 w-2 rounded-full ${dot}`} />
            {status}
          </span>
          <button onClick={() => setOpen(false)} className="text-white/50 hover:text-white" aria-label="Collapse">—</button>
        </div>
      </header>

      <div className="overflow-y-auto">
        <section className="border-b border-white/10 px-4 py-3">
          <div className="mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-white/45">Browsing as</div>
          <div className="grid grid-cols-2 gap-1.5">
            {people.map((p) => (
              <button
                key={p.id}
                onClick={() => onPerson(p.id)}
                className={`rounded-lg px-2.5 py-1.5 text-left text-xs transition ${person === p.id ? "bg-[#e8e4da] text-ink" : "bg-white/5 hover:bg-white/10"}`}
              >
                <span className="block font-semibold">{p.name}</span>
                <span className={`block truncate text-[11px] ${person === p.id ? "text-ink-2" : "text-white/45"}`}>{p.note}</span>
              </button>
            ))}
          </div>
          <div className="mt-2 flex items-center justify-between gap-2 font-mono text-[11px] text-white/55">
            <span className="truncate">
              key <span className="text-white/85">{ctx.key}</span>
              {Object.entries(ctx.attributes ?? {})
                .filter(([k]) => k !== "email")
                .map(([k, v]) => <span key={k}> · {k} <span className="text-white/85">{String(v)}</span></span>)}
            </span>
            {person === "visitor" && (
              <button onClick={onNewVisitor} className="shrink-0 rounded bg-white/10 px-2 py-0.5 text-white/80 hover:bg-white/20">new visitor</button>
            )}
          </div>
        </section>

        <section className="px-4 py-3">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-[0.18em] text-white/45">Flags in this environment</span>
            <label className="flex cursor-pointer items-center gap-1.5 text-[11px] text-white/60">
              <input type="checkbox" checked={xray} onChange={(e) => onXray(e.target.checked)} className="accent-[#c9973f]" /> x-ray
            </label>
          </div>
          <ul className="space-y-1">
            {Object.entries(flags)
              .sort(([a], [b]) => a.localeCompare(b))
              .map(([key, r]) => (
                <li key={key}>
                  <a href={`${dashboard}/flags/${key}`} target="_blank" rel="noreferrer" className="group flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-white/5" title="Open in the Flagpole dashboard">
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-mono text-xs text-white/90 group-hover:underline">{key}</span>
                      <span className="block text-[11px] text-white/40">{why(r)}</span>
                    </span>
                    <span className={`rounded px-1.5 py-0.5 font-mono text-[11px] ${r.reason === "off" ? "bg-white/8 text-white/50" : "bg-[#c9973f]/25 text-[#f0c879]"}`}>
                      {r.variant}
                      {typeof r.value !== "boolean" && r.value !== r.variant && <span className="text-white/45"> = {JSON.stringify(r.value)}</span>}
                    </span>
                  </a>
                </li>
              ))}
          </ul>
        </section>

        <section className="border-t border-white/10 px-4 py-3">
          <div className="mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-white/45">Changes received</div>
          {log.length === 0 ? (
            <p className="text-[11px] leading-relaxed text-white/45">Change a flag in the dashboard's Development tab and it lands here, and on the page, in about a second.</p>
          ) : (
            <ul className="space-y-1.5">
              {log.map((l, i) => (
                <li key={l.at.getTime() + i} className={`font-mono text-[11px] ${i === 0 ? "slide-in text-white/85" : "text-white/45"}`}>
                  <span className="text-white/40">{l.at.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })}</span>{" "}
                  {l.flags.map((f) => `${f.key} → ${f.variant}`).join(", ")}
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </aside>
  );
}
