import { Link, Outlet, useLocation, useNavigate, useParams } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import { allows, useChangeRequests, useMe, useProjects } from "../api/queries";
import { Badge } from "./ui";

export function Mark({ className = "" }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={className} aria-hidden="true">
      <rect x="5" y="2" width="2.5" height="28" rx="1" className="fill-navy" />
      <circle cx="6.25" cy="2.4" r="1.9" className="fill-navy" />
      <path d="M7.5 4.5h18v11h-18z" fill="#f2b705" />
      <path d="M7.5 4.5h18v11z" fill="#c8102e" />
    </svg>
  );
}

export function Wordmark() {
  return (
    <span className="flex items-center gap-2">
      <Mark className="h-7 w-7" />
      <span className="font-display text-2xl font-extrabold uppercase tracking-[0.08em] text-ink">Flagpole</span>
    </span>
  );
}

function ThemeToggle() {
  const [theme, setTheme] = useState(() => document.documentElement.dataset.theme ?? "");
  const next = theme === "dark" ? "light" : theme === "light" ? "" : "dark";
  const label = theme === "" ? "Auto" : theme === "dark" ? "Dark" : "Light";
  return (
    <button
      className="rounded-md px-2 py-1 text-xs text-muted hover:bg-surface-2 hover:text-ink"
      onClick={() => {
        if (next) document.documentElement.dataset.theme = next;
        else delete document.documentElement.dataset.theme;
        try {
          if (next) localStorage.setItem("flagpole-theme", next);
          else localStorage.removeItem("flagpole-theme");
        } catch {
          /* private mode */
        }
        setTheme(next);
      }}
      title="Switch theme"
    >
      ◐ {label}
    </button>
  );
}

function ProjectNav({ project }: { project: string }) {
  const { data: me } = useMe();
  const { data: changes } = useChangeRequests(project);
  const pending = changes?.filter((c) => c.status === "pending").length ?? 0;
  const item = "flex items-center justify-between rounded-md px-3 py-1.5 text-sm text-ink-2 hover:bg-surface-2 hover:text-ink";
  const active = { className: "bg-surface-2 text-ink font-semibold" };
  return (
    <div className="mt-1 ml-3 space-y-0.5 border-l border-line pl-2">
      <Link to="/projects/$project" params={{ project }} activeOptions={{ exact: true }} className={item} activeProps={active}>
        Flags
      </Link>
      <Link to="/projects/$project/changes" params={{ project }} className={item} activeProps={active}>
        Changes
        {pending > 0 && <span className="rounded-full bg-signal-yellow px-1.5 text-[11px] font-bold text-navy" style={{ color: "#0b2545" }}>{pending}</span>}
      </Link>
      <Link to="/projects/$project/audit" params={{ project }} className={item} activeProps={active}>
        Audit log
      </Link>
      {allows(me?.role, "admin") && (
        <Link to="/projects/$project/settings" params={{ project }} className={item} activeProps={active}>
          Settings
        </Link>
      )}
    </div>
  );
}

export function Shell() {
  const { data: me } = useMe();
  const { data: projects } = useProjects();
  const params = useParams({ strict: false }) as { project?: string };
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const [menu, setMenu] = useState(false);
  useEffect(() => setMenu(false), [pathname]);

  async function signOut() {
    await api.POST("/api/v1/auth/logout");
    qc.clear();
    navigate({ to: "/login", search: { next: undefined } });
  }

  return (
    <div className="min-h-screen md:flex">
      <header className="sticky top-0 z-30 flex items-center justify-between border-b border-line bg-surface/90 px-4 py-3 backdrop-blur md:hidden">
        <Link to="/"><Wordmark /></Link>
        <button onClick={() => setMenu(!menu)} aria-expanded={menu} aria-label="Menu" className="rounded-md border border-line-2 px-2.5 py-1 text-sm">
          {menu ? "✕" : "☰"}
        </button>
      </header>
      {menu && <div className="fixed inset-0 z-30 bg-navy/30 md:hidden" onClick={() => setMenu(false)} />}
      <aside
        className={`fixed inset-y-0 left-0 z-40 flex w-64 shrink-0 flex-col border-r border-line bg-surface transition-transform md:sticky md:top-0 md:h-screen md:translate-x-0 md:bg-surface/80 md:backdrop-blur ${menu ? "translate-x-0" : "-translate-x-full"}`}
      >
        <div className="px-5 pt-5 pb-4">
          <Link to="/"><Wordmark /></Link>
        </div>
        <nav className="flex-1 overflow-y-auto px-3">
          <Link to="/" activeOptions={{ exact: true }} className="mb-3 block rounded-md px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.16em] text-muted hover:text-ink" activeProps={{ className: "text-ink" }}>
            Projects
          </Link>
          {projects?.map((p) => (
            <div key={p.key} className="mb-1">
              <Link
                to="/projects/$project"
                params={{ project: p.key }}
                className={`flex items-center gap-2 rounded-md px-3 py-1.5 text-sm hover:bg-surface-2 ${params.project === p.key ? "font-semibold text-ink" : "text-ink-2"}`}
              >
                <span className={`h-2 w-2 rounded-sm ${params.project === p.key ? "bg-signal-red" : "bg-line-2"}`} />
                {p.name}
              </Link>
              {params.project === p.key && <ProjectNav project={p.key} />}
            </div>
          ))}
          {allows(me?.role, "admin") && (
            <Link to="/users" className="mt-4 block rounded-md px-3 py-1.5 text-sm text-ink-2 hover:bg-surface-2 hover:text-ink" activeProps={{ className: "bg-surface-2 font-semibold text-ink" }}>
              Users
            </Link>
          )}
          <a href="/docs" target="_blank" rel="noreferrer" className="mt-1 block rounded-md px-3 py-1.5 text-sm text-ink-2 hover:bg-surface-2 hover:text-ink">
            API reference ↗
          </a>
        </nav>
        <div className="border-t border-line p-4">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold">{me?.name}</div>
              <div className="truncate text-xs text-muted">{me?.email}</div>
            </div>
            {me && <Badge tone={me.role}>{me.role}</Badge>}
          </div>
          <div className="mt-3 flex items-center justify-between">
            <ThemeToggle />
            <button onClick={signOut} className="rounded-md px-2 py-1 text-xs text-muted hover:bg-surface-2 hover:text-ink">Sign out</button>
          </div>
        </div>
      </aside>
      <main className="min-w-0 flex-1 px-4 py-6 md:px-10 md:py-8">
        <div className="mx-auto max-w-6xl">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
