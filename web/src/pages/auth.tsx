import { useState, type FormEvent, type ReactNode } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "../api/client";
import { fetchMe, keys } from "../api/queries";
import { Button, Field, Input, ProblemNote } from "../components/ui";
import { Mark } from "../components/shell";

// FLAGPOLE spelled in the International Code of Signals, hoisted top to bottom.
const B = "#1f4e9a", R = "#c8102e", Y = "#f2b705", W = "#ffffff", K = "#101828";
const spelling: [string, ReactNode][] = [
  ["F", <><rect width="30" height="20" fill={W} /><path d="M15 2 L27 10 L15 18 L3 10Z" fill={R} /></>],
  ["L", <><rect width="15" height="10" fill={Y} /><rect x="15" width="15" height="10" fill={K} /><rect y="10" width="15" height="10" fill={K} /><rect x="15" y="10" width="15" height="10" fill={Y} /></>],
  ["A", <><rect width="14" height="20" fill={W} /><path d="M14 0 H30 L23 10 L30 20 H14Z" fill={B} /></>],
  ["G", <>{[0, 1, 2, 3, 4, 5].map((i) => <rect key={i} x={i * 5} width="5" height="20" fill={i % 2 ? B : Y} />)}</>],
  ["P", <><rect width="30" height="20" fill={B} /><rect x="8" y="5.5" width="14" height="9" fill={W} /></>],
  ["O", <><rect width="30" height="20" fill={Y} /><path d="M0 0 H30 V20Z" fill={R} /></>],
  ["L", <><rect width="15" height="10" fill={Y} /><rect x="15" width="15" height="10" fill={K} /><rect y="10" width="15" height="10" fill={K} /><rect x="15" y="10" width="15" height="10" fill={Y} /></>],
  ["E", <><rect width="30" height="10" fill={B} /><rect y="10" width="30" height="10" fill={R} /></>],
];

function Halyard() {
  return (
    <div className="relative flex h-full items-center justify-center" aria-hidden="true">
      <div className="relative flex">
        <div className="mr-1 w-[5px] rounded-full bg-navy" style={{ height: spelling.length * 56 + 40 }} />
        <ul className="space-y-3 pt-6">
          {spelling.map(([letter, draw], i) => (
            <li key={i} className="rise flex items-center gap-4" style={{ animationDelay: `${i * 90}ms` }}>
              <span className="h-px w-3 bg-ink-2" />
              <svg width="66" height="44" viewBox="0 0 30 20" className="rounded-[2px] shadow-[0_6px_14px_-6px_rgb(16_24_40/0.5)]">{draw}</svg>
              <span className="font-display text-3xl font-extrabold text-ink-2">{letter}</span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function AuthFrame({ title, lede, children }: { title: string; lede: string; children: ReactNode }) {
  return (
    <div className="grid min-h-screen lg:grid-cols-[1.1fr_1fr]">
      <section className="relative hidden overflow-hidden border-r border-line bg-surface/70 lg:block">
        <div className="absolute top-8 left-10 flex items-center gap-2">
          <Mark className="h-8 w-8" />
          <span className="font-display text-3xl font-extrabold uppercase tracking-[0.08em]">Flagpole</span>
        </div>
        <Halyard />
        <p className="absolute bottom-8 left-10 max-w-sm text-sm text-muted">
          Feature flags with roles, reviewed changes for production, and an SDK that streams every change as it happens.
        </p>
      </section>
      <section className="flex items-center justify-center px-6 py-12">
        <div className="w-full max-w-sm">
          <h1 className="font-display text-5xl font-extrabold uppercase leading-none tracking-wide">{title}</h1>
          <p className="mt-2 mb-8 text-sm text-muted">{lede}</p>
          {children}
        </div>
      </section>
    </div>
  );
}

const demo = [
  ["ada@example.com", "admin"],
  ["linus@example.com", "approver"],
  ["grace@example.com", "editor"],
  ["vera@example.com", "viewer"],
];

export function LoginPage() {
  const { next } = useSearch({ from: "/login" });
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await fetchMe().catch(() => undefined); // picks up the CSRF cookie
      const me = await unwrap(api.POST("/api/v1/auth/login", { body: { email, password } }));
      qc.setQueryData(keys.me, me);
      navigate({ to: next && next.startsWith("/") ? next : "/" });
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthFrame title="Sign in" lede="Run up a flag, review a change, or see who saw what.">
      <form onSubmit={submit} className="space-y-4">
        <Field label="Email"><Input type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus /></Field>
        <Field label="Password"><Input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required /></Field>
        <ProblemNote error={error} />
        <Button variant="primary" type="submit" disabled={busy} className="w-full py-2">{busy ? "Signing in…" : "Sign in"}</Button>
        <Link to="/forgot-password" className="block text-center text-xs text-muted hover:text-ink">Forgot your password?</Link>
      </form>
      {import.meta.env.DEV && (
        <div className="mt-10 rounded-lg border border-dashed border-line-2 p-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-muted">Demo accounts · flagpole-demo-password</div>
          <div className="mt-2 grid grid-cols-2 gap-1.5">
            {demo.map(([e, r]) => (
              <button key={e} type="button" onClick={() => { setEmail(e); setPassword("flagpole-demo-password"); }} className="rounded-md border border-line px-2 py-1 text-left text-xs hover:bg-surface-2">
                <span className="font-semibold capitalize">{r}</span>
                <span className="block truncate text-muted">{e}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </AuthFrame>
  );
}

function PasswordForm({ submitLabel, onSubmit }: { submitLabel: string; onSubmit: (password: string) => Promise<void> }) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  return (
    <form
      className="space-y-4"
      onSubmit={async (e) => {
        e.preventDefault();
        setBusy(true);
        setError(null);
        try {
          await fetchMe().catch(() => undefined);
          await onSubmit(password);
        } catch (err) {
          setError(err);
        } finally {
          setBusy(false);
        }
      }}
    >
      <Field label="New password" hint="At least 12 characters.">
        <Input type="password" autoComplete="new-password" minLength={12} value={password} onChange={(e) => setPassword(e.target.value)} required autoFocus />
      </Field>
      <ProblemNote error={error} />
      <Button variant="primary" type="submit" disabled={busy} className="w-full py-2">{submitLabel}</Button>
    </form>
  );
}

export function AcceptInvitePage() {
  const { token } = useSearch({ from: "/accept-invite" });
  const navigate = useNavigate();
  const qc = useQueryClient();
  return (
    <AuthFrame title="Welcome aboard" lede="Choose a password to finish setting up your account.">
      <PasswordForm
        submitLabel="Set password and sign in"
        onSubmit={async (password) => {
          const me = await unwrap(api.POST("/api/v1/auth/invites/accept", { body: { token, password } }));
          qc.setQueryData(keys.me, me);
          navigate({ to: "/" });
        }}
      />
    </AuthFrame>
  );
}

export function ResetPasswordPage() {
  const { token } = useSearch({ from: "/reset-password" });
  const [done, setDone] = useState(false);
  return (
    <AuthFrame title="New password" lede="Setting it signs you out on every device.">
      {done ? (
        <p className="text-sm">Password changed. <Link to="/login" search={{ next: undefined }} className="font-semibold underline">Sign in</Link>.</p>
      ) : (
        <PasswordForm
          submitLabel="Change password"
          onSubmit={async (password) => {
            await unwrap(api.POST("/api/v1/auth/password-reset/confirm", { body: { token, password } }));
            setDone(true);
          }}
        />
      )}
    </AuthFrame>
  );
}

export function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<unknown>(null);
  return (
    <AuthFrame title="Reset" lede="We'll email you a link that works for an hour.">
      {sent ? (
        <p className="text-sm">If <strong>{email}</strong> has an account, a reset link is on its way. Locally, it's in Mailpit at <a className="underline" href="http://localhost:8025">localhost:8025</a>.</p>
      ) : (
        <form
          className="space-y-4"
          onSubmit={async (e) => {
            e.preventDefault();
            try {
              await fetchMe().catch(() => undefined);
              await unwrap(api.POST("/api/v1/auth/password-reset", { body: { email } }));
              setSent(true);
            } catch (err) {
              setError(err);
            }
          }}
        >
          <Field label="Email"><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus /></Field>
          <ProblemNote error={error} />
          <Button variant="primary" type="submit" className="w-full py-2">Send reset link</Button>
          <Link to="/login" search={{ next: undefined }} className="block text-center text-xs text-muted hover:text-ink">Back to sign in</Link>
        </form>
      )}
    </AuthFrame>
  );
}
