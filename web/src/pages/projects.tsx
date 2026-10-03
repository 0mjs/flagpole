import { useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { api, unwrap } from "../api/client";
import { allows, keys, useAction, useMe, useProjects } from "../api/queries";
import { SignalFlag } from "../components/signal";
import { Button, Dialog, Empty, Field, Input, Loading, PageHeader, ProblemNote, relative } from "../components/ui";

export function ProjectsPage() {
  const { data: me } = useMe();
  const { data: projects, isLoading } = useProjects();
  const [open, setOpen] = useState(false);

  return (
    <>
      <PageHeader eyebrow={`Signed in as ${me?.name ?? ""}`} title="Projects">
        {allows(me?.role, "admin") && <Button variant="primary" onClick={() => setOpen(true)}>New project</Button>}
      </PageHeader>
      {isLoading ? (
        <Loading />
      ) : !projects?.length ? (
        <Empty title="No projects yet">An admin creates a project; it comes with development, staging and production.</Empty>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {projects.map((p, i) => (
            <Link
              key={p.key}
              to="/projects/$project"
              params={{ project: p.key }}
              className="panel rise group relative block overflow-hidden p-5 transition hover:-translate-y-0.5"
              style={{ animationDelay: `${i * 60}ms` }}
            >
              <div className="absolute inset-y-0 left-0 w-1 bg-signal-red opacity-0 transition group-hover:opacity-100" />
              <div className="mono text-xs text-muted">{p.key}</div>
              <div className="mt-1 font-display text-3xl font-extrabold uppercase tracking-wide">{p.name}</div>
              <p className="mt-1 min-h-10 text-sm text-ink-2">{p.description || "No description."}</p>
              <div className="mt-4 flex items-center justify-between">
                <div className="flex gap-1.5">
                  <SignalFlag env="development" size={14} />
                  <SignalFlag env="staging" size={14} />
                  <SignalFlag env="production" size={14} />
                </div>
                <span className="text-xs text-muted">Created {relative(p.created_at)}</span>
              </div>
            </Link>
          ))}
        </div>
      )}
      <Dialog open={open} onClose={() => setOpen(false)} title="New project">
        <NewProject onDone={() => setOpen(false)} />
      </Dialog>
    </>
  );
}

function NewProject({ onDone }: { onDone: () => void }) {
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [touched, setTouched] = useState(false);
  const [description, setDescription] = useState("");
  const slug = touched ? key : name.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  const create = useAction(
    () => unwrap(api.POST("/api/v1/projects", { body: { key: slug, name, description } })),
    [keys.projects],
  );
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(undefined, {
          onSuccess: (p) => {
            onDone();
            navigate({ to: "/projects/$project", params: { project: p.key } });
          },
        });
      }}
    >
      <Field label="Name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
      <Field label="Key" hint="Used in URLs and the API. Lowercase letters, digits and hyphens.">
        <Input className="mono" value={slug} onChange={(e) => { setTouched(true); setKey(e.target.value); }} required />
      </Field>
      <Field label="Description"><Input value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
      <p className="text-xs text-muted">It starts with development, staging and production. Production changes need an approver.</p>
      <ProblemNote error={create.error} />
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onDone}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={create.isPending}>Create project</Button>
      </div>
    </form>
  );
}
