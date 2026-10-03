import { useState } from "react";
import { useParams } from "@tanstack/react-router";
import { api, unwrap, type Schemas } from "../api/client";
import { useAudit } from "../api/queries";
import { Button, Empty, Loading, PageHeader } from "../components/ui";

type Event = Schemas["AuditEvent"];

// Each action gets a mark in the margin, like a ship's log.
function mark(action: string): [string, string] {
  if (action === "change.approved" || action === "change.applied") return ["✓", "text-go"];
  if (/(rejected|conflicted|revoked|archived|disabled)$/.test(action)) return ["✕", "text-signal-red"];
  if (action.startsWith("change.")) return ["?", "text-signal-blue"];
  if (action.startsWith("flag.config")) return ["⚑", "text-signal-yellow"];
  return ["•", "text-muted"];
}

export function AuditPage() {
  const { project } = useParams({ from: "/app/projects/$project/audit" });
  const { data, isLoading } = useAudit(project);
  const [older, setOlder] = useState<Event[]>([]);
  const [before, setBefore] = useState<number | null | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const cursor = before === undefined ? data?.before : before;
  const events = [...(data?.events ?? []), ...older];

  async function more() {
    if (!cursor) return;
    setLoading(true);
    try {
      const page = await unwrap(api.GET("/api/v1/projects/{project}/audit", { params: { path: { project }, query: { before: cursor, limit: 100 } } }));
      setOlder([...older, ...(page.events ?? [])]);
      setBefore(page.before ?? null);
    } finally {
      setLoading(false);
    }
  }

  // Group by day, newest first.
  const days = new Map<string, Event[]>();
  for (const e of events) {
    const d = new Date(e.created_at).toLocaleDateString(undefined, { weekday: "long", day: "numeric", month: "long" });
    days.set(d, [...(days.get(d) ?? []), e]);
  }

  return (
    <>
      <PageHeader eyebrow="Every change, who made it, and when" title="Audit log" />
      {isLoading ? (
        <Loading />
      ) : events.length === 0 ? (
        <Empty title="Nothing logged yet" />
      ) : (
        <div className="space-y-8">
          {[...days].map(([day, list]) => (
            <section key={day}>
              <h2 className="mb-2 font-display text-lg font-extrabold uppercase tracking-[0.12em] text-muted">{day}</h2>
              <ol className="panel divide-y divide-line">
                {list.map((e) => {
                  const [glyph, tone] = mark(e.action);
                  return (
                    <li key={e.id} className="grid grid-cols-[3.5rem_1.5rem_1fr] items-baseline gap-2 px-4 py-2.5 text-sm sm:grid-cols-[3.5rem_1.5rem_1fr_auto]">
                      <time className="mono text-xs text-muted" dateTime={e.created_at}>
                        {new Date(e.created_at).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })}
                      </time>
                      <span className={`text-center font-bold ${tone}`}>{glyph}</span>
                      <span className="min-w-0">{e.summary}</span>
                      <code className="mono hidden text-[11px] text-muted sm:block">{e.action}</code>
                    </li>
                  );
                })}
              </ol>
            </section>
          ))}
          {cursor ? <Button onClick={more} disabled={loading}>{loading ? "Loading…" : "Older entries"}</Button> : <p className="text-center text-xs text-muted">That's the start of the log.</p>}
        </div>
      )}
    </>
  );
}
