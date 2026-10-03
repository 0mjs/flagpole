import { useState } from "react";
import { api, unwrap } from "../api/client";
import { keys, useAction, useMe, useUsers, type Role, type User } from "../api/queries";
import { Badge, Button, Dialog, Field, Input, Loading, PageHeader, ProblemNote, Select, relative } from "../components/ui";

const roles: [Role, string][] = [
  ["viewer", "Sees flags, changes and the audit log."],
  ["editor", "Creates flags and changes them where no approval is needed; requests changes elsewhere."],
  ["approver", "Everything an editor can, and approves other people's change requests."],
  ["admin", "Everything, plus users, projects, environments and SDK keys."],
];

export function UsersPage() {
  const { data: users, isLoading } = useUsers();
  const [inviting, setInviting] = useState(false);

  return (
    <>
      <PageHeader eyebrow="Crew" title="Users">
        <Button variant="primary" onClick={() => setInviting(true)}>Invite</Button>
      </PageHeader>
      <div className="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {roles.map(([r, text]) => (
          <div key={r} className="panel p-3">
            <Badge tone={r}>{r}</Badge>
            <p className="mt-2 text-xs text-ink-2">{text}</p>
          </div>
        ))}
      </div>
      {isLoading ? (
        <Loading />
      ) : (
        <div className="panel overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-line text-left text-xs uppercase tracking-wide text-muted">
                <th className="px-4 py-3 font-semibold">Person</th>
                <th className="px-4 py-3 font-semibold">Role</th>
                <th className="px-4 py-3 font-semibold">Status</th>
                <th className="px-4 py-3 font-semibold">Joined</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody>{(users ?? []).map((u) => <UserRow key={u.id} user={u} />)}</tbody>
          </table>
        </div>
      )}
      <Dialog open={inviting} onClose={() => setInviting(false)} title="Invite someone">
        <Invite onDone={() => setInviting(false)} />
      </Dialog>
    </>
  );
}

function UserRow({ user: u }: { user: User }) {
  const { data: me } = useMe();
  const self = me?.id === u.id;
  const role = useAction((role: Role) => unwrap(api.PATCH("/api/v1/users/{id}", { params: { path: { id: u.id } }, body: { role } })), [keys.users]);
  const toggle = useAction(
    () =>
      u.status === "disabled"
        ? unwrap(api.POST("/api/v1/users/{id}/enable", { params: { path: { id: u.id } } }))
        : unwrap(api.POST("/api/v1/users/{id}/disable", { params: { path: { id: u.id } } })),
    [keys.users],
  );
  const signOut = useAction(() => unwrap(api.DELETE("/api/v1/users/{id}/sessions", { params: { path: { id: u.id } } })), []);
  const error = role.error ?? toggle.error ?? signOut.error;

  return (
    <>
      <tr className={`border-b border-line last:border-0 ${u.status === "disabled" ? "opacity-55" : ""}`}>
        <td className="px-4 py-3">
          <div className="font-semibold">{u.name}{self && <span className="ml-1 text-xs font-normal text-muted">(you)</span>}</div>
          <div className="text-xs text-muted">{u.email}</div>
        </td>
        <td className="px-4 py-3">
          {self ? (
            <Badge tone={u.role}>{u.role}</Badge>
          ) : (
            <Select value={u.role} disabled={role.isPending} onChange={(e) => role.mutate(e.target.value as Role)} className="w-32 py-1">
              {roles.map(([r]) => <option key={r} value={r}>{r}</option>)}
            </Select>
          )}
        </td>
        <td className="px-4 py-3">
          <span className={`inline-flex items-center gap-1.5 text-xs ${u.status === "active" ? "text-go" : u.status === "invited" ? "text-signal-blue" : "text-signal-red"}`}>
            <span className="h-1.5 w-1.5 rounded-full bg-current" />
            {u.status}
          </span>
        </td>
        <td className="px-4 py-3 text-xs text-muted">{relative(u.created_at)}</td>
        <td className="px-4 py-3 text-right whitespace-nowrap">
          {!self && (
            <>
              {u.status === "active" && <Button variant="ghost" disabled={signOut.isPending} onClick={() => signOut.mutate()}>{signOut.isSuccess ? "Signed out" : "Sign out everywhere"}</Button>}
              <Button variant="ghost" className={u.status === "disabled" ? "" : "text-signal-red"} disabled={toggle.isPending} onClick={() => toggle.mutate()}>
                {u.status === "disabled" ? "Enable" : "Disable"}
              </Button>
            </>
          )}
        </td>
      </tr>
      {error != null && (
        <tr><td colSpan={5} className="px-4 pb-3"><ProblemNote error={error} /></td></tr>
      )}
    </>
  );
}

function Invite({ onDone }: { onDone: () => void }) {
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState<Role>("editor");
  const invite = useAction(() => unwrap(api.POST("/api/v1/users", { body: { email, name, role } })), [keys.users]);

  if (invite.isSuccess) {
    return (
      <div className="space-y-4">
        <p className="text-sm">Invite sent to <strong>{email}</strong>. The link works for 7 days. Locally, it's in Mailpit at <a className="underline" href="http://localhost:8025" target="_blank" rel="noreferrer">localhost:8025</a>.</p>
        <div className="flex justify-end"><Button variant="primary" onClick={onDone}>Done</Button></div>
      </div>
    );
  }
  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); invite.mutate(); }}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
        <Field label="Email"><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required /></Field>
      </div>
      <fieldset>
        <legend className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-2">Role</legend>
        <div className="space-y-1.5">
          {roles.map(([r, text]) => (
            <label key={r} className={`flex cursor-pointer items-start gap-3 rounded-md border p-2.5 ${role === r ? "border-navy bg-surface-2" : "border-line"}`}>
              <input type="radio" name="role" value={r} checked={role === r} onChange={() => setRole(r)} className="mt-1" />
              <span>
                <span className="text-sm font-semibold capitalize">{r}</span>
                <span className="block text-xs text-muted">{text}</span>
              </span>
            </label>
          ))}
        </div>
      </fieldset>
      <ProblemNote error={invite.error} />
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onDone}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={invite.isPending}>Send invite</Button>
      </div>
    </form>
  );
}
