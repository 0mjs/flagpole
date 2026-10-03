import { useState } from "react";
import { Link, useParams } from "@tanstack/react-router";
import { api, unwrap, type Schemas } from "../api/client";
import { allows, keys, useAction, useChangeRequests, useMe, type ChangeRequest } from "../api/queries";
import { SignalFlag } from "../components/signal";
import { Button, Empty, Input, Loading, PageHeader, ProblemNote, relative } from "../components/ui";

type Status = Schemas["ChangeStatus"];
const tabs: [Status | "all", string][] = [["pending", "Waiting"], ["applied", "Applied"], ["rejected", "Rejected"], ["conflicted", "Conflicted"], ["cancelled", "Withdrawn"], ["all", "All"]];
const stamp: Record<Status, string> = {
  pending: "border-signal-yellow text-ink bg-signal-yellow/15",
  applied: "border-go text-go",
  rejected: "border-signal-red text-signal-red",
  conflicted: "border-signal-red text-signal-red border-dashed",
  cancelled: "border-line-2 text-muted",
};

export function ChangesPage() {
  const { project } = useParams({ from: "/app/projects/$project/changes" });
  const { data: changes, isLoading } = useChangeRequests(project);
  const [tab, setTab] = useState<Status | "all">("pending");
  const shown = (changes ?? []).filter((c) => tab === "all" || c.status === tab);

  return (
    <>
      <PageHeader eyebrow="Separation of duties" title="Changes" />
      <p className="-mt-3 mb-5 max-w-2xl text-sm text-ink-2">
        Changes to environments that need approval wait here. An approver or admin applies them, and nobody approves their own. If the config moved on since a request was made, approving it marks it conflicted instead.
      </p>
      <div className="mb-5 flex gap-1 overflow-x-auto border-b border-line">
        {tabs.map(([k, label]) => {
          const n = (changes ?? []).filter((c) => k === "all" || c.status === k).length;
          return (
            <button key={k} onClick={() => setTab(k)} className={`-mb-px border-b-2 px-3 py-2 text-sm whitespace-nowrap ${tab === k ? "border-signal-red font-semibold" : "border-transparent text-ink-2 hover:text-ink"}`}>
              {label} <span className="mono text-xs text-muted">{n}</span>
            </button>
          );
        })}
      </div>
      {isLoading ? (
        <Loading />
      ) : shown.length === 0 ? (
        <Empty title={tab === "pending" ? "Nothing waiting" : "None"}>{tab === "pending" ? "The queue is clear." : "No change requests with this status."}</Empty>
      ) : (
        <div className="space-y-3">
          {shown.map((c, i) => <ChangeCard key={c.id} project={project} change={c} delay={i * 40} />)}
        </div>
      )}
    </>
  );
}

function ChangeCard({ project, change: c, delay }: { project: string; change: ChangeRequest; delay: number }) {
  const { data: me } = useMe();
  const [comment, setComment] = useState("");
  const path = { project, id: c.id };
  const invalidate = [keys.changes(project), keys.flag(project, c.flag), keys.flags(project), keys.audit(project)];
  const approve = useAction(() => unwrap(api.POST("/api/v1/projects/{project}/change-requests/{id}/approve", { params: { path }, body: { comment } })), invalidate);
  const reject = useAction(() => unwrap(api.POST("/api/v1/projects/{project}/change-requests/{id}/reject", { params: { path }, body: { comment } })), invalidate);
  const cancel = useAction(() => unwrap(api.POST("/api/v1/projects/{project}/change-requests/{id}/cancel", { params: { path } })), invalidate);

  const mine = me?.id === c.author.id;
  const canReview = c.status === "pending" && allows(me?.role, "approver") && !mine;
  const canCancel = c.status === "pending" && (mine || allows(me?.role, "admin"));
  const busy = approve.isPending || reject.isPending || cancel.isPending;
  const p = c.proposed;

  return (
    <article className="panel rise relative overflow-hidden" style={{ animationDelay: `${delay}ms` }}>
      <div className="flex flex-wrap items-start gap-4 p-5">
        <SignalFlag env={c.environment} size={22} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-baseline gap-x-2">
            <Link to="/projects/$project/flags/$flag" params={{ project, flag: c.flag }} className="mono font-semibold hover:underline">{c.flag}</Link>
            <span className="text-sm text-muted">in</span>
            <span className="text-sm font-semibold">{c.environment}</span>
          </div>
          <div className="mt-0.5 text-sm text-ink-2">
            {c.author.name}{mine && " (you)"} · {relative(c.created_at)} · from v{c.base_version}
          </div>
          {c.comment && <blockquote className="mt-2 border-l-2 border-line-2 pl-3 text-sm italic text-ink-2">{c.comment}</blockquote>}
          <dl className="mt-3 flex flex-wrap gap-x-6 gap-y-1 text-sm">
            <div><dt className="inline text-muted">Turns it </dt><dd className={`inline font-semibold ${p.enabled ? "text-go" : "text-signal-red"}`}>{p.enabled ? "on" : "off"}</dd></div>
            <div><dt className="inline text-muted">Default </dt><dd className="mono inline">{p.default_variant}</dd></div>
            <div><dt className="inline text-muted">Off </dt><dd className="mono inline">{p.off_variant}</dd></div>
            <div><dt className="inline text-muted">Rules </dt><dd className="mono inline">{p.rules?.length ?? 0}</dd></div>
          </dl>
          {(p.rules ?? []).length > 0 && (
            <ol className="mt-2 space-y-1 text-xs text-ink-2">
              {(p.rules ?? []).map((r, i) => (
                <li key={i} className="mono">
                  {i + 1}. {(r.conditions ?? []).map((x) => `${x.attribute} ${x.operator} [${(x.values ?? []).join(", ")}]`).join(" and ") || "everyone"} →{" "}
                  {r.rollout?.length ? r.rollout.map((w) => `${w.variant} ${w.weight}%`).join(" / ") : r.variant}
                </li>
              ))}
            </ol>
          )}
          {c.reviewer && (
            <div className="mt-3 text-sm text-ink-2">
              {c.status === "cancelled" ? "Withdrawn" : c.status === "applied" ? "Approved" : "Reviewed"} by <strong>{c.reviewer.name}</strong>
              {c.reviewed_at && ` · ${relative(c.reviewed_at)}`}
              {c.review_comment && <span className="italic"> · "{c.review_comment}"</span>}
            </div>
          )}
        </div>
        <span className={`rotate-[-4deg] rounded border-2 px-2 py-0.5 font-display text-sm font-extrabold uppercase tracking-[0.14em] ${stamp[c.status]}`}>
          {c.status === "cancelled" ? "withdrawn" : c.status}
        </span>
      </div>
      {(canReview || canCancel) && (
        <div className="flex flex-wrap items-center gap-2 border-t border-line bg-surface-2/40 px-5 py-3">
          {canReview && <Input value={comment} onChange={(e) => setComment(e.target.value)} placeholder="Comment (optional)" className="min-w-48 flex-1" />}
          {canReview && <Button variant="danger" disabled={busy} onClick={() => reject.mutate()}>Reject</Button>}
          {canReview && <Button variant="primary" disabled={busy} onClick={() => approve.mutate()}>Approve and apply</Button>}
          {canCancel && <Button variant="ghost" disabled={busy} onClick={() => cancel.mutate()}>Withdraw</Button>}
          {c.status === "pending" && mine && !canReview && <span className="text-xs text-muted">Someone else has to approve this.</span>}
        </div>
      )}
      {c.status === "pending" && !canReview && !canCancel && <div className="border-t border-line px-5 py-2 text-xs text-muted">Waiting for an approver.</div>}
      {[approve.error, reject.error, cancel.error].map((e, i) => e != null && <div key={i} className="px-5 pb-4"><ProblemNote error={e} /></div>)}
    </article>
  );
}
