import { useState } from "react";
import { useParams } from "@tanstack/react-router";
import { api, unwrap, type Schemas } from "../api/client";
import { keys, useAction, useProject, useSDKKeys } from "../api/queries";
import { SignalFlag, flagFor } from "../components/signal";
import { Button, Dialog, Field, Input, Loading, PageHeader, ProblemNote, Switch, relative } from "../components/ui";

type Environment = Schemas["Environment"];

export function SettingsPage() {
  const { project } = useParams({ from: "/app/projects/$project/settings" });
  const { data: detail, isLoading } = useProject(project);
  const [adding, setAdding] = useState(false);
  if (isLoading || !detail) return <Loading />;

  return (
    <>
      <PageHeader eyebrow={detail.name} title="Settings" />
      <ProjectDetails project={project} name={detail.name} description={detail.description} />
      <div className="mt-8 mb-3 flex items-end justify-between">
        <div>
          <h2 className="font-display text-2xl font-extrabold uppercase tracking-wide">Environments</h2>
          <p className="text-sm text-muted">Each one has its own flag configs and SDK keys.</p>
        </div>
        <Button onClick={() => setAdding(true)}>Add environment</Button>
      </div>
      <div className="space-y-4">
        {(detail.environments ?? []).map((e) => <EnvironmentPanel key={e.key} project={project} env={e} />)}
      </div>
      <Dialog open={adding} onClose={() => setAdding(false)} title="Add environment">
        <NewEnvironment project={project} onDone={() => setAdding(false)} />
      </Dialog>
    </>
  );
}

function ProjectDetails({ project, name: initialName, description: initialDescription }: { project: string; name: string; description: string }) {
  const [name, setName] = useState(initialName);
  const [description, setDescription] = useState(initialDescription);
  const save = useAction(() => unwrap(api.PATCH("/api/v1/projects/{project}", { params: { path: { project } }, body: { name, description } })), [keys.project(project), keys.projects]);
  return (
    <form className="panel grid gap-4 p-5 sm:grid-cols-[1fr_2fr_auto] sm:items-end" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <Field label="Name"><Input value={name} onChange={(e) => setName(e.target.value)} required /></Field>
      <Field label="Description"><Input value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
      <Button type="submit" disabled={save.isPending || (name === initialName && description === initialDescription)}>Save</Button>
      {save.error != null && <div className="sm:col-span-3"><ProblemNote error={save.error} /></div>}
    </form>
  );
}

function EnvironmentPanel({ project, env }: { project: string; env: Environment }) {
  const f = flagFor(env.key);
  const update = useAction(
    (requires_approval: boolean) => unwrap(api.PATCH("/api/v1/projects/{project}/environments/{env}", { params: { path: { project, env: env.key } }, body: { name: env.name, requires_approval } })),
    [keys.project(project)],
  );
  return (
    <section className="panel overflow-hidden">
      <div className="flex flex-wrap items-center gap-4 border-b border-line p-5">
        <SignalFlag env={env.key} size={28} />
        <div className="flex-1">
          <div className="font-display text-2xl font-extrabold uppercase tracking-wide">{env.name}</div>
          <div className="text-xs text-muted"><span className="mono">{env.key}</span> · flies {f.letter}, "{f.meaning}"</div>
        </div>
        <label className="flex items-center gap-3 text-sm">
          <span className="text-right">
            <span className="block font-semibold">Require approval</span>
            <span className="block text-xs text-muted">Flag changes go through review</span>
          </span>
          <Switch on={env.requires_approval} disabled={update.isPending} onChange={(v) => update.mutate(v)} label={`Require approval in ${env.name}`} />
        </label>
      </div>
      <ProblemNote error={update.error} />
      <SDKKeys project={project} env={env.key} />
    </section>
  );
}

function SDKKeys({ project, env }: { project: string; env: string }) {
  const { data: sdkKeys } = useSDKKeys(project, env);
  const [name, setName] = useState("");
  const [created, setCreated] = useState<Schemas["NewSDKKey"] | null>(null);
  const [copied, setCopied] = useState(false);
  const path = { project, env };
  const create = useAction(() => unwrap(api.POST("/api/v1/projects/{project}/environments/{env}/sdk-keys", { params: { path }, body: { name } })), [keys.sdkKeys(project, env)]);
  const revoke = useAction((id: string) => unwrap(api.DELETE("/api/v1/projects/{project}/environments/{env}/sdk-keys/{id}", { params: { path: { ...path, id } } })), [keys.sdkKeys(project, env)]);

  return (
    <div className="p-5">
      <h3 className="text-xs font-semibold uppercase tracking-wide text-muted">SDK keys</h3>
      {created && (
        <div className="mt-3 rounded-lg border border-go/50 bg-go/8 p-4">
          <div className="text-sm font-semibold">Copy {created.name} now. It won't be shown again.</div>
          <div className="mt-2 flex gap-2">
            <code className="mono flex-1 overflow-x-auto rounded-md border border-line bg-surface px-3 py-2 text-xs whitespace-nowrap">{created.key}</code>
            <Button onClick={() => { navigator.clipboard?.writeText(created.key); setCopied(true); }}>{copied ? "Copied" : "Copy"}</Button>
            <Button variant="ghost" onClick={() => { setCreated(null); setCopied(false); }}>Done</Button>
          </div>
        </div>
      )}
      <ul className="mt-3 divide-y divide-line">
        {(sdkKeys ?? []).map((k) => (
          <li key={k.id} className={`flex flex-wrap items-center gap-3 py-2.5 text-sm ${k.revoked_at ? "opacity-50" : ""}`}>
            <span className="font-semibold">{k.name}</span>
            <code className="mono text-xs text-muted">{k.prefix}…</code>
            <span className="ml-auto text-xs text-muted">{k.revoked_at ? `Revoked ${relative(k.revoked_at)}` : `Created ${relative(k.created_at)}`}</span>
            {!k.revoked_at && <Button variant="ghost" className="text-signal-red" disabled={revoke.isPending} onClick={() => revoke.mutate(k.id)}>Revoke</Button>}
          </li>
        ))}
        {sdkKeys?.length === 0 && <li className="py-3 text-sm text-muted">No keys yet.</li>}
      </ul>
      <form
        className="mt-3 flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate(undefined, { onSuccess: (k) => { setCreated(k); setCopied(false); setName(""); } });
        }}
      >
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Name, such as checkout-service" required className="max-w-xs" />
        <Button type="submit" disabled={create.isPending}>Create key</Button>
      </form>
      <ProblemNote error={create.error ?? revoke.error} />
    </div>
  );
}

function NewEnvironment({ project, onDone }: { project: string; onDone: () => void }) {
  const [name, setName] = useState("");
  const [requiresApproval, setRequiresApproval] = useState(false);
  const key = name.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  const create = useAction(
    () => unwrap(api.POST("/api/v1/projects/{project}/environments", { params: { path: { project } }, body: { key, name, requires_approval: requiresApproval } })),
    [keys.project(project), keys.flags(project)],
  );
  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); create.mutate(undefined, { onSuccess: onDone }); }}>
      <Field label="Name" hint={key ? `Key: ${key}` : "Such as QA or Preview."}>
        <Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
      </Field>
      {key && (
        <div className="flex items-center gap-3 text-sm text-ink-2">
          <SignalFlag env={key} size={18} /> It will fly {flagFor(key).letter}, "{flagFor(key).meaning}".
        </div>
      )}
      <label className="flex items-center gap-3 text-sm">
        <Switch on={requiresApproval} onChange={setRequiresApproval} label="Require approval" /> Require approval for flag changes
      </label>
      <p className="text-xs text-muted">Every flag starts off here, serving the same variants as the first environment, with no rules.</p>
      <ProblemNote error={create.error} />
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onDone}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={create.isPending}>Add environment</Button>
      </div>
    </form>
  );
}
