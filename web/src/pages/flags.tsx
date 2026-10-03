import { useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { api, unwrap, type Schemas } from "../api/client";
import { allows, keys, useAction, useFlags, useMe, useProject } from "../api/queries";
import { Hoist, SignalFlag } from "../components/signal";
import { Button, Dialog, Empty, Field, Input, Loading, PageHeader, ProblemNote, Select, relative } from "../components/ui";

export function FlagsPage() {
  const { project } = useParams({ from: "/app/projects/$project/" });
  const { data: me } = useMe();
  const { data: detail } = useProject(project);
  const [archived, setArchived] = useState(false);
  const { data: flags, isLoading } = useFlags(project, archived);
  const [query, setQuery] = useState("");
  const [tag, setTag] = useState("");
  const [open, setOpen] = useState(false);

  const envs = detail?.environments ?? [];
  const tags = useMemo(() => [...new Set((flags ?? []).flatMap((f) => f.tags ?? []))].sort(), [flags]);
  const shown = (flags ?? []).filter(
    (f) => (!tag || f.tags?.includes(tag)) && (!query || f.key.includes(query.toLowerCase()) || f.name.toLowerCase().includes(query.toLowerCase())),
  );

  return (
    <>
      <PageHeader eyebrow={detail?.name ?? project} title="Flags">
        {allows(me?.role, "editor") && <Button variant="primary" onClick={() => setOpen(true)}>New flag</Button>}
      </PageHeader>

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Input placeholder="Search by key or name" value={query} onChange={(e) => setQuery(e.target.value)} className="max-w-xs" />
        <div className="flex flex-wrap gap-1">
          {tags.map((t) => (
            <button
              key={t}
              onClick={() => setTag(tag === t ? "" : t)}
              className={`rounded-full border px-2.5 py-1 text-xs ${tag === t ? "border-navy bg-navy text-paper" : "border-line-2 text-ink-2 hover:bg-surface-2"}`}
            >
              #{t}
            </button>
          ))}
        </div>
        <label className="ml-auto flex items-center gap-2 text-xs text-muted">
          <input type="checkbox" checked={archived} onChange={(e) => setArchived(e.target.checked)} /> Show archived
        </label>
      </div>

      {isLoading ? (
        <Loading />
      ) : !flags?.length ? (
        <Empty title="No flags here">Create a flag, then turn it on environment by environment.</Empty>
      ) : (
        <div className="panel overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-line text-left text-xs uppercase tracking-wide text-muted">
                <th className="px-4 py-3 font-semibold">Flag</th>
                {envs.map((e) => (
                  <th key={e.key} className="px-3 py-3 font-semibold">
                    <span className="flex items-center gap-2"><SignalFlag env={e.key} size={12} />{e.name}</span>
                  </th>
                ))}
                <th className="px-4 py-3 text-right font-semibold">Updated</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((f, i) => (
                <tr key={f.key} className={`rise border-b border-line last:border-0 hover:bg-surface-2/60 ${f.archived_at ? "opacity-55" : ""}`} style={{ animationDelay: `${i * 30}ms` }}>
                  <td className="px-4 py-3">
                    <Link to="/projects/$project/flags/$flag" params={{ project, flag: f.key }} className="group block">
                      <span className="font-semibold group-hover:underline">{f.name}</span>
                      <span className="mono block text-xs text-muted sm:ml-2 sm:inline">{f.key}</span>
                      <span className="mt-1 flex flex-wrap gap-1">
                        <span className="rounded bg-surface-2 px-1.5 text-[11px] text-ink-2">{f.kind}</span>
                        {f.tags?.map((t) => <span key={t} className="text-[11px] text-muted">#{t}</span>)}
                        {f.archived_at && <span className="rounded bg-signal-red/12 px-1.5 text-[11px] text-signal-red">archived</span>}
                      </span>
                    </Link>
                  </td>
                  {envs.map((e) => {
                    const s = f.environments?.[e.key];
                    return <td key={e.key} className="px-3 py-2">{s ? <Hoist env={e.key} on={s.enabled} rules={s.rules} /> : <span className="text-muted">—</span>}</td>;
                  })}
                  <td className="px-4 py-3 text-right text-xs text-muted">{relative(f.updated_at)}</td>
                </tr>
              ))}
              {shown.length === 0 && (
                <tr><td colSpan={envs.length + 2} className="px-4 py-8 text-center text-sm text-muted">No flags match.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      )}

      <Dialog open={open} onClose={() => setOpen(false)} title="New flag">
        <NewFlag project={project} onDone={() => setOpen(false)} />
      </Dialog>
    </>
  );
}

type Kind = Schemas["Kind"];
const samples: Record<Exclude<Kind, "boolean">, [string, string][]> = {
  string: [["control", '"blue"'], ["treatment", '"green"']],
  number: [["low", "50"], ["high", "100"]],
  json: [["off", "{}"], ["on", '{"banner": "Hello"}']],
};

function NewFlag({ project, onDone }: { project: string; onDone: () => void }) {
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [touched, setTouched] = useState(false);
  const [description, setDescription] = useState("");
  const [kind, setKind] = useState<Kind>("boolean");
  const [tags, setTags] = useState("");
  const [variants, setVariants] = useState<[string, string][]>([]);
  const [parseError, setParseError] = useState("");
  const slug = touched ? key : name.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");

  const create = useAction(
    (body: Schemas["CreateFlagInput"]) => unwrap(api.POST("/api/v1/projects/{project}/flags", { params: { path: { project } }, body })),
    [keys.flags(project)],
  );

  function submit() {
    setParseError("");
    let parsed: Schemas["VariantInput"][] | undefined;
    if (kind !== "boolean") {
      try {
        parsed = variants.map(([k, v]) => ({ key: k, value: JSON.parse(v) }));
      } catch {
        setParseError("Each variant value must be JSON, such as \"blue\", 42 or {\"a\": 1}.");
        return;
      }
    }
    create.mutate(
      { key: slug, name, description, kind, tags: tags.split(",").map((t) => t.trim()).filter(Boolean), variants: parsed },
      {
        onSuccess: (f) => {
          onDone();
          navigate({ to: "/projects/$project/flags/$flag", params: { project, flag: f.key } });
        },
      },
    );
  }

  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); submit(); }}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
        <Field label="Key"><Input className="mono" value={slug} onChange={(e) => { setTouched(true); setKey(e.target.value); }} required /></Field>
      </div>
      <Field label="Description"><Input value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Kind">
          <Select value={kind} onChange={(e) => { const k = e.target.value as Kind; setKind(k); setVariants(k === "boolean" ? [] : samples[k]); }}>
            <option value="boolean">Boolean (on / off)</option>
            <option value="string">String</option>
            <option value="number">Number</option>
            <option value="json">JSON</option>
          </Select>
        </Field>
        <Field label="Tags" hint="Comma separated."><Input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="checkout, experiment" /></Field>
      </div>
      {kind !== "boolean" && (
        <div>
          <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-ink-2">Variants</div>
          <div className="space-y-2">
            {variants.map(([k, v], i) => (
              <div key={i} className="flex gap-2">
                <Input className="mono w-40" value={k} onChange={(e) => setVariants(variants.map((x, j) => (j === i ? [e.target.value, x[1]] : x)))} placeholder="key" />
                <Input className="mono" value={v} onChange={(e) => setVariants(variants.map((x, j) => (j === i ? [x[0], e.target.value] : x)))} placeholder="JSON value" />
                <Button type="button" variant="ghost" onClick={() => setVariants(variants.filter((_, j) => j !== i))} aria-label="Remove variant">✕</Button>
              </div>
            ))}
            <Button type="button" onClick={() => setVariants([...variants, ["", ""]])}>Add variant</Button>
          </div>
        </div>
      )}
      {parseError && <ProblemNote error={parseError} />}
      <ProblemNote error={create.error} />
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onDone}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={create.isPending}>Create flag</Button>
      </div>
    </form>
  );
}
