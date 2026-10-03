import { useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { api, unwrap, type Schemas } from "../api/client";
import { allows, keys, useAction, useChangeRequests, useExposures, useFlag, useMe, useProject, type EnvironmentConfig, type Flag, type Rule } from "../api/queries";
import { Hoist, SignalFlag, flagFor } from "../components/signal";
import { Badge, Button, Dialog, Field, Input, Loading, PageHeader, ProblemNote, Select, Switch, relative } from "../components/ui";

type Condition = Schemas["Condition"];
type Operator = Schemas["Operator"];
type Draft = { enabled: boolean; default_variant: string; off_variant: string; rules: Rule[] };

const operators: [Operator, string][] = [
  ["in", "is one of"],
  ["not_in", "is not one of"],
  ["contains", "contains"],
  ["starts_with", "starts with"],
  ["ends_with", "ends with"],
  ["gt", ">"],
  ["gte", "≥"],
  ["lt", "<"],
  ["lte", "≤"],
];
const palette = ["var(--signal-blue)", "var(--signal-red)", "var(--signal-yellow)", "var(--go)", "var(--muted)", "var(--ink-2)"];

const signalColours: Record<string, string> = {
  blue: "var(--signal-blue)",
  red: "var(--signal-red)",
  green: "var(--go)",
  yellow: "var(--signal-yellow)",
  orange: "#e8772e",
  black: "var(--ink)",
  grey: "var(--muted)",
  gray: "var(--muted)",
};

// A variant whose value is a colour, such as "green", is drawn in that colour.
function colourOf(flag: Flag | undefined, variant: string): string {
  const variants = flag?.variants ?? [];
  const i = Math.max(0, variants.findIndex((v) => v.key === variant));
  const value = variants[i]?.value;
  if (typeof value === "string") {
    const named = signalColours[value.toLowerCase()];
    if (named) return named;
    if (/^#[0-9a-f]{3,8}$/i.test(value)) return value;
  }
  return palette[i % palette.length];
}

export function FlagPage() {
  const { project, flag: flagKey } = useParams({ from: "/app/projects/$project/flags/$flag" });
  const { data: flag, isLoading, error } = useFlag(project, flagKey);
  const { data: detail } = useProject(project);
  const [env, setEnv] = useState<string>("");

  if (isLoading) return <Loading />;
  if (!flag) return <ProblemNote error={error} />;

  const envs = detail?.environments ?? [];
  const current = env || envs[0]?.key || flag.environments?.[0]?.environment || "";
  const cfg = flag.environments?.find((c) => c.environment === current);
  const environment = envs.find((e) => e.key === current);

  return (
    <>
      <FlagHeader project={project} flag={flag} />

      <div className="grid gap-6 lg:grid-cols-[1fr_300px]">
        <div className="min-w-0">
          <div role="tablist" className="flex gap-1 overflow-x-auto border-b border-line">
            {envs.map((e) => {
              const c = flag.environments?.find((x) => x.environment === e.key);
              const active = e.key === current;
              return (
                <button
                  key={e.key}
                  role="tab"
                  aria-selected={active}
                  onClick={() => setEnv(e.key)}
                  className={`-mb-px flex items-center gap-2.5 border-b-2 px-4 py-2.5 text-sm whitespace-nowrap ${active ? "border-signal-red font-semibold text-ink" : "border-transparent text-ink-2 hover:text-ink"}`}
                >
                  <SignalFlag env={e.key} size={14} />
                  {e.name}
                  {c && <span className={`h-1.5 w-1.5 rounded-full ${c.enabled ? "bg-go" : "bg-line-2"}`} />}
                </button>
              );
            })}
          </div>

          {cfg && environment && (
            <ConfigEditor
              key={`${current}:${cfg.version}`}
              project={project}
              flag={flag}
              cfg={cfg}
              envName={environment.name}
              requiresApproval={environment.requires_approval}
            />
          )}

          {cfg && <ExposureChart project={project} flag={flag.key} env={current} />}
        </div>

        <aside className="space-y-4">
          <Variants flag={flag} />
          <section className="panel p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-muted">Across environments</h3>
            <ul className="mt-3 space-y-2">
              {envs.map((e) => {
                const c = flag.environments?.find((x) => x.environment === e.key);
                return (
                  <li key={e.key} className="flex items-center justify-between">
                    <span className="text-sm">{e.name}</span>
                    {c && <Hoist env={e.key} on={c.enabled} rules={c.rules?.length ?? 0} />}
                  </li>
                );
              })}
            </ul>
          </section>
          <section className="panel p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-muted">In your code</h3>
            <pre className="mono mt-2 overflow-x-auto rounded-md bg-surface-2 p-3 text-[11px] leading-relaxed text-ink-2">{`POST /sdk/v1/evaluate
Authorization: Bearer fp_…

{ "flags": ["${flag.key}"],
  "context": { "key": "user-42" } }`}</pre>
          </section>
        </aside>
      </div>
    </>
  );
}

function FlagHeader({ project, flag }: { project: string; flag: Flag }) {
  const { data: me } = useMe();
  const navigate = useNavigate();
  const [editing, setEditing] = useState(false);
  const canEdit = allows(me?.role, "editor");
  const toggle = useAction(
    () =>
      flag.archived_at
        ? unwrap(api.POST("/api/v1/projects/{project}/flags/{flag}/restore", { params: { path: { project, flag: flag.key } } }))
        : unwrap(api.POST("/api/v1/projects/{project}/flags/{flag}/archive", { params: { path: { project, flag: flag.key } } })),
    [keys.flag(project, flag.key), keys.flags(project)],
  );
  return (
    <>
      <Link to="/projects/$project" params={{ project }} className="mb-3 inline-block text-xs text-muted hover:text-ink">← All flags</Link>
      <PageHeader
        eyebrow={
          <span className="flex items-center gap-2 normal-case tracking-normal">
            <span className="mono">{flag.key}</span>
            <span className="rounded bg-surface-2 px-1.5 text-[11px] uppercase tracking-wide">{flag.kind}</span>
            {flag.archived_at && <Badge tone="admin">archived</Badge>}
          </span>
        }
        title={flag.name}
      >
        {canEdit && (
          <>
            <Button onClick={() => setEditing(true)}>Edit details</Button>
            <Button
              variant={flag.archived_at ? "default" : "ghost"}
              disabled={toggle.isPending}
              onClick={() => toggle.mutate(undefined, { onSuccess: () => !flag.archived_at && navigate({ to: "/projects/$project", params: { project } }) })}
            >
              {flag.archived_at ? "Restore" : "Archive"}
            </Button>
          </>
        )}
      </PageHeader>
      {(flag.description || flag.tags?.length) && (
        <p className="-mt-3 mb-6 max-w-3xl text-sm text-ink-2">
          {flag.description}
          {flag.tags?.map((t) => <span key={t} className="ml-2 text-xs text-muted">#{t}</span>)}
        </p>
      )}
      <ProblemNote error={toggle.error} />
      <Dialog open={editing} onClose={() => setEditing(false)} title="Edit flag">
        <EditDetails project={project} flag={flag} onDone={() => setEditing(false)} />
      </Dialog>
    </>
  );
}

function EditDetails({ project, flag, onDone }: { project: string; flag: Flag; onDone: () => void }) {
  const [name, setName] = useState(flag.name);
  const [description, setDescription] = useState(flag.description);
  const [tags, setTags] = useState((flag.tags ?? []).join(", "));
  const save = useAction(
    () =>
      unwrap(
        api.PATCH("/api/v1/projects/{project}/flags/{flag}", {
          params: { path: { project, flag: flag.key } },
          body: { name, description, tags: tags.split(",").map((t) => t.trim()).filter(Boolean) },
        }),
      ),
    [keys.flag(project, flag.key), keys.flags(project)],
  );
  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate(undefined, { onSuccess: onDone }); }}>
      <Field label="Name"><Input value={name} onChange={(e) => setName(e.target.value)} required /></Field>
      <Field label="Description"><Input value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
      <Field label="Tags" hint="Comma separated."><Input value={tags} onChange={(e) => setTags(e.target.value)} /></Field>
      <ProblemNote error={save.error} />
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onDone}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={save.isPending}>Save</Button>
      </div>
    </form>
  );
}

function Variants({ flag }: { flag: Flag }) {
  return (
    <section className="panel p-4">
      <h3 className="text-xs font-semibold uppercase tracking-wide text-muted">Variants</h3>
      <ul className="mt-3 space-y-2">
        {(flag.variants ?? []).map((v) => (
          <li key={v.key} className="flex items-center gap-2 text-sm">
            <span className="h-2.5 w-2.5 shrink-0 rounded-sm" style={{ background: colourOf(flag, v.key) }} />
            <span className="font-semibold">{v.key}</span>
            <code className="mono ml-auto truncate text-xs text-muted">{JSON.stringify(v.value)}</code>
          </li>
        ))}
      </ul>
    </section>
  );
}

const draftOf = (c: EnvironmentConfig): Draft => ({ enabled: c.enabled, default_variant: c.default_variant, off_variant: c.off_variant, rules: c.rules ?? [] });
const toInput = (rules: Rule[]): Schemas["RuleInput"][] =>
  rules.map((r) => ({
    description: r.description || undefined,
    conditions: r.conditions ?? [],
    ...(r.rollout?.length ? { rollout: r.rollout } : { variant: r.variant }),
  }));

function ConfigEditor({ project, flag, cfg, envName, requiresApproval }: { project: string; flag: Flag; cfg: EnvironmentConfig; envName: string; requiresApproval: boolean }) {
  const { data: me } = useMe();
  const { data: changes } = useChangeRequests(project);
  const [draft, setDraft] = useState<Draft>(() => draftOf(cfg));
  const [comment, setComment] = useState("");
  const canEdit = allows(me?.role, "editor") && !flag.archived_at;
  const dirty = JSON.stringify(draft) !== JSON.stringify(draftOf(cfg));
  const pending = changes?.find((c) => c.status === "pending" && c.flag === flag.key && c.environment === cfg.environment);
  const variants = (flag.variants ?? []).map((v) => v.key);
  const path = { project, flag: flag.key, env: cfg.environment };
  const invalidate = [keys.flag(project, flag.key), keys.flags(project), keys.changes(project), keys.audit(project)];

  const save = useAction(
    (): Promise<unknown> =>
      requiresApproval
        ? unwrap(api.POST("/api/v1/projects/{project}/flags/{flag}/environments/{env}/change-requests", { params: { path }, body: { ...draft, rules: toInput(draft.rules), base_version: cfg.version, comment } }))
        : unwrap(api.PUT("/api/v1/projects/{project}/flags/{flag}/environments/{env}", { params: { path }, body: { ...draft, rules: toInput(draft.rules), base_version: cfg.version } })),
    invalidate,
  );

  const set = (patch: Partial<Draft>) => setDraft({ ...draft, ...patch });
  const setRule = (i: number, r: Rule) => set({ rules: draft.rules.map((x, j) => (j === i ? r : x)) });
  const f = flagFor(cfg.environment);

  return (
    <div className="mt-5 space-y-5">
      {requiresApproval && (
        <div className="flex items-start gap-3 rounded-lg border border-signal-yellow/50 bg-signal-yellow/10 px-4 py-3 text-sm">
          <SignalFlag env={cfg.environment} size={16} />
          <div>
            <strong>{envName} flies {f.letter}, "{f.meaning}".</strong> Changes here go to an approver before they apply; you can't approve your own.
          </div>
        </div>
      )}
      {pending && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-signal-blue/40 bg-signal-blue/8 px-4 py-3 text-sm">
          <span>
            <strong>{pending.author.name}</strong> asked {relative(pending.created_at)} to turn it <strong>{pending.proposed.enabled ? "on" : "off"}</strong>
            {pending.comment && <span className="text-ink-2"> · "{pending.comment}"</span>}
          </span>
          <Link to="/projects/$project/changes" params={{ project }} className="font-semibold text-signal-blue hover:underline">Review →</Link>
        </div>
      )}

      <section className="panel p-5">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-center gap-4">
            <Switch on={draft.enabled} onChange={(v) => set({ enabled: v })} disabled={!canEdit} label={`Serve ${flag.name} in ${envName}`} />
            <div>
              <div className="font-display text-2xl font-extrabold uppercase tracking-wide">{draft.enabled ? "Hoisted" : "Lowered"}</div>
              <div className="text-xs text-muted">{draft.enabled ? "Rules apply, then the default." : `Everyone gets ${draft.off_variant}.`}</div>
            </div>
          </div>
          <div className="mono text-xs text-muted">v{cfg.version} · {relative(cfg.updated_at)}</div>
        </div>
        <div className="mt-5 grid gap-4 sm:grid-cols-2">
          <Field label="When on, by default serve">
            <Select value={draft.default_variant} disabled={!canEdit} onChange={(e) => set({ default_variant: e.target.value })}>
              {variants.map((v) => <option key={v}>{v}</option>)}
            </Select>
          </Field>
          <Field label="When off, serve">
            <Select value={draft.off_variant} disabled={!canEdit} onChange={(e) => set({ off_variant: e.target.value })}>
              {variants.map((v) => <option key={v}>{v}</option>)}
            </Select>
          </Field>
        </div>
      </section>

      <section>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="font-display text-xl font-extrabold uppercase tracking-wide">Targeting rules</h3>
          <span className="text-xs text-muted">Checked top to bottom; the first match wins.</span>
        </div>
        <div className="space-y-3">
          {draft.rules.map((r, i) => (
            <RuleCard
              key={i}
              index={i}
              rule={r}
              flag={flag}
              variants={variants}
              disabled={!canEdit}
              onChange={(nr) => setRule(i, nr)}
              onRemove={() => set({ rules: draft.rules.filter((_, j) => j !== i) })}
              onMove={(d) => {
                const rules = [...draft.rules];
                const j = i + d;
                if (j < 0 || j >= rules.length) return;
                [rules[i], rules[j]] = [rules[j], rules[i]];
                set({ rules });
              }}
            />
          ))}
          {draft.rules.length === 0 && <div className="rounded-lg border border-dashed border-line-2 px-4 py-6 text-center text-sm text-muted">No rules. Everyone gets the default when it's on.</div>}
          {canEdit && (
            <Button onClick={() => set({ rules: [...draft.rules, { conditions: [{ attribute: "country", operator: "in", values: [] }], variant: variants[0] }] })}>
              Add rule
            </Button>
          )}
        </div>
      </section>

      {canEdit && dirty && (
        <div className="rise sticky bottom-4 z-10 flex flex-wrap items-center gap-3 rounded-xl border border-line bg-surface/95 p-3 shadow-lg backdrop-blur">
          {requiresApproval && <Input value={comment} onChange={(e) => setComment(e.target.value)} placeholder="Why? The approver will see this." className="min-w-56 flex-1" />}
          {!requiresApproval && <span className="flex-1 text-sm text-ink-2">Unsaved changes to {envName}.</span>}
          <Button variant="ghost" onClick={() => setDraft(draftOf(cfg))}>Discard</Button>
          <Button variant="primary" disabled={save.isPending || (requiresApproval && !!pending)} onClick={() => save.mutate(undefined, { onSuccess: () => setComment("") })}>
            {requiresApproval ? (pending ? "A change is already pending" : "Request change") : "Save"}
          </Button>
          {save.error != null && <div className="w-full"><ProblemNote error={save.error} /></div>}
        </div>
      )}
      {save.isSuccess && requiresApproval && !dirty && <p className="text-sm text-go">Change requested. An approver will review it.</p>}
    </div>
  );
}

function RuleCard({ index, rule, flag, variants, disabled, onChange, onRemove, onMove }: { index: number; rule: Rule; flag: Flag; variants: string[]; disabled: boolean; onChange: (r: Rule) => void; onRemove: () => void; onMove: (d: number) => void }) {
  const conditions = rule.conditions ?? [];
  const mode = rule.rollout?.length ? "rollout" : "variant";
  const setCond = (i: number, c: Condition) => onChange({ ...rule, conditions: conditions.map((x, j) => (j === i ? c : x)) });
  const total = (rule.rollout ?? []).reduce((s, w) => s + w.weight, 0);

  return (
    <div className="panel relative overflow-hidden p-4 pl-14">
      <div className="absolute inset-y-0 left-0 flex w-10 flex-col items-center justify-center gap-1 border-r border-line bg-surface-2/60">
        <span className="font-display text-xl font-extrabold text-ink-2">{index + 1}</span>
        {!disabled && (
          <>
            <button className="text-xs text-muted hover:text-ink" onClick={() => onMove(-1)} aria-label="Move up">▲</button>
            <button className="text-xs text-muted hover:text-ink" onClick={() => onMove(1)} aria-label="Move down">▼</button>
          </>
        )}
      </div>
      <div className="flex items-center gap-2">
        <Input value={rule.description ?? ""} disabled={disabled} onChange={(e) => onChange({ ...rule, description: e.target.value })} placeholder="Describe the rule (optional)" className="border-transparent bg-transparent px-0 font-semibold focus:px-3" />
        {!disabled && <Button variant="ghost" onClick={onRemove} aria-label="Remove rule">✕</Button>}
      </div>
      <div className="mt-2 space-y-2">
        {conditions.length === 0 && <div className="flex items-center gap-2 text-sm text-ink-2"><span className="mono w-10 text-right text-xs text-muted">if</span>everyone</div>}
        {conditions.map((c, i) => (
          <div key={i} className="flex flex-wrap items-center gap-2">
            <span className="mono w-10 text-right text-xs text-muted">{i === 0 ? "if" : "and"}</span>
            <Input className="mono w-32" value={c.attribute} disabled={disabled} onChange={(e) => setCond(i, { ...c, attribute: e.target.value })} placeholder="attribute" />
            <Select className="w-36" value={c.operator} disabled={disabled} onChange={(e) => setCond(i, { ...c, operator: e.target.value as Operator })}>
              {operators.map(([op, label]) => <option key={op} value={op}>{label}</option>)}
            </Select>
            <Input
              className="mono min-w-40 flex-1"
              defaultValue={(c.values ?? []).join(", ")}
              disabled={disabled}
              onBlur={(e) => setCond(i, { ...c, values: e.target.value.split(",").map((v) => v.trim()).filter(Boolean) })}
              placeholder="values, comma separated"
            />
            {!disabled && <button className="text-xs text-muted hover:text-signal-red" onClick={() => onChange({ ...rule, conditions: conditions.filter((_, j) => j !== i) })} aria-label="Remove condition">✕</button>}
          </div>
        ))}
        {!disabled && (
          <button className="ml-12 text-xs font-semibold text-signal-blue hover:underline" onClick={() => onChange({ ...rule, conditions: [...conditions, { attribute: "", operator: "in", values: [] }] })}>
            + condition
          </button>
        )}
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-line pt-3">
        <span className="mono w-10 text-right text-xs text-muted">then</span>
        <Select
          className="w-44"
          value={mode}
          disabled={disabled}
          onChange={(e) =>
            onChange(
              e.target.value === "rollout"
                ? { ...rule, variant: undefined, rollout: variants.map((v, i) => ({ variant: v, weight: i === 0 ? 100 - Math.floor(100 / variants.length) * (variants.length - 1) : Math.floor(100 / variants.length) })) }
                : { ...rule, rollout: undefined, variant: variants[0] },
            )
          }
        >
          <option value="variant">serve</option>
          <option value="rollout">split by percentage</option>
        </Select>
        {mode === "variant" ? (
          <Select className="w-44" value={rule.variant} disabled={disabled} onChange={(e) => onChange({ ...rule, variant: e.target.value })}>
            {variants.map((v) => <option key={v}>{v}</option>)}
          </Select>
        ) : (
          <div className="flex flex-1 flex-wrap items-center gap-3">
            {(rule.rollout ?? []).map((w, i) => (
              <label key={w.variant} className="flex items-center gap-1.5 text-sm">
                <span className="h-2.5 w-2.5 rounded-sm" style={{ background: colourOf(flag, w.variant) }} />
                {w.variant}
                <Input
                  type="number"
                  min={0}
                  max={100}
                  className="mono w-20"
                  value={w.weight}
                  disabled={disabled}
                  onChange={(e) => onChange({ ...rule, rollout: (rule.rollout ?? []).map((x, j) => (j === i ? { ...x, weight: Number(e.target.value) } : x)) })}
                />
                %
              </label>
            ))}
            <span className={`mono text-xs ${total === 100 ? "text-go" : "text-signal-red"}`}>{total}/100</span>
          </div>
        )}
      </div>
      {mode === "rollout" && (
        <div className="mt-3 flex h-1.5 overflow-hidden rounded-full bg-surface-2">
          {(rule.rollout ?? []).map((w) => <div key={w.variant} style={{ width: `${w.weight}%`, background: colourOf(flag, w.variant) }} />)}
        </div>
      )}
    </div>
  );
}

function ExposureChart({ project, flag, env }: { project: string; flag: string; env: string }) {
  const [hours, setHours] = useState(24);
  const { data } = useExposures(project, flag, env, hours);
  const { data: full } = useFlag(project, flag);
  const variantKeys = (full?.variants ?? []).map((v) => v.key);

  const series = useMemo(() => {
    if (!data) return [];
    const byHour = new Map((data.buckets ?? []).map((b) => [new Date(b.at).getTime(), b.counts ?? {}]));
    const end = new Date(data.until);
    end.setMinutes(0, 0, 0);
    return Array.from({ length: hours }, (_, i) => {
      const at = end.getTime() - (hours - 1 - i) * 3_600_000;
      return { at, counts: byHour.get(at) ?? {} };
    });
  }, [data, hours]);

  const max = Math.max(1, ...series.map((s) => Object.values(s.counts).reduce((a, b) => a + b, 0)));
  const totals = data?.totals ?? {};
  const sum = Object.values(totals).reduce((a, b) => a + b, 0);
  const W = 720, H = 160, gap = hours > 48 ? 0.5 : 2;
  const bw = W / Math.max(series.length, 1);

  return (
    <section className="panel mt-6 p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="font-display text-xl font-extrabold uppercase tracking-wide">Exposures</h3>
          <p className="text-xs text-muted">Which variant SDKs served, by hour. {sum.toLocaleString()} in the last {hours === 168 ? "7 days" : `${hours} hours`}.</p>
        </div>
        <div className="flex rounded-md border border-line-2 p-0.5 text-xs">
          {[24, 48, 168].map((h) => (
            <button key={h} onClick={() => setHours(h)} className={`rounded px-2.5 py-1 ${hours === h ? "bg-navy text-paper" : "text-ink-2 hover:bg-surface-2"}`}>
              {h === 168 ? "7d" : `${h}h`}
            </button>
          ))}
        </div>
      </div>
      {sum === 0 ? (
        <div className="mt-4 rounded-lg border border-dashed border-line-2 py-10 text-center text-sm text-muted">Nothing served yet. SDKs report exposures to POST /sdk/v1/exposures.</div>
      ) : (
        <>
          <svg viewBox={`0 0 ${W} ${H + 18}`} className="mt-4 w-full" role="img" aria-label="Exposures by hour">
            {[0.25, 0.5, 0.75, 1].map((t) => <line key={t} x1="0" x2={W} y1={H - H * t} y2={H - H * t} stroke="var(--line)" strokeDasharray="2 4" />)}
            {series.map((s, i) => {
              let y = H;
              return (
                <g key={s.at}>
                  <title>{`${new Date(s.at).toLocaleString(undefined, { weekday: "short", hour: "numeric" })}: ${Object.entries(s.counts).map(([k, v]) => `${k} ${v}`).join(", ") || "none"}`}</title>
                  <rect x={i * bw} y={0} width={bw} height={H} fill="transparent" />
                  {variantKeys.map((v) => {
                    const n = s.counts[v] ?? 0;
                    if (!n) return null;
                    const h = (n / max) * H;
                    y -= h;
                    return <rect key={v} x={i * bw + gap / 2} y={y} width={Math.max(bw - gap, 0.5)} height={h} fill={colourOf(full, v)} />;
                  })}
                </g>
              );
            })}
            {series.map((s, i) =>
              i % Math.ceil(series.length / 6) === 0 ? (
                <text key={s.at} x={i * bw} y={H + 14} fontSize="10" fill="var(--muted)" fontFamily="var(--font-mono)">
                  {new Date(s.at).toLocaleString(undefined, hours > 48 ? { weekday: "short" } : { hour: "numeric" })}
                </text>
              ) : null,
            )}
          </svg>
          <div className="mt-3 flex flex-wrap gap-4 text-sm">
            {variantKeys.map((v) => (
              <span key={v} className="flex items-center gap-1.5">
                <span className="h-2.5 w-2.5 rounded-sm" style={{ background: colourOf(full, v) }} />
                {v}
                <span className="mono text-xs text-muted">{(totals[v] ?? 0).toLocaleString()} · {sum ? Math.round(((totals[v] ?? 0) / sum) * 100) : 0}%</span>
              </span>
            ))}
          </div>
        </>
      )}
    </section>
  );
}
