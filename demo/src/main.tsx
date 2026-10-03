import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { FlagpoleClient } from "@flagpole/sdk";
import { FlagpoleProvider } from "@flagpole/sdk/react";
import { App } from "./App";
import { people } from "./people";
import "./styles.css";

const url = import.meta.env.VITE_FLAGPOLE_URL ?? "http://localhost:8080";

function storedKey(): string {
  try {
    return localStorage.getItem("harbour-sdk-key") ?? "";
  } catch {
    return "";
  }
}

function Root() {
  const [key, setKey] = useState<string>(() => import.meta.env.VITE_FLAGPOLE_KEY || storedKey());
  const [client, setClient] = useState<FlagpoleClient | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!key) return;
    const person = (() => {
      try {
        return localStorage.getItem("harbour-person") ?? "ada";
      } catch {
        return "ada";
      }
    })();
    const c = new FlagpoleClient({ url, key, context: (people.find((p) => p.id === person) ?? people[0]).context() });
    let cancelled = false;
    c.start().then(
      () => !cancelled && setClient(c),
      (err: Error) => !cancelled && setError(err.message),
    );
    return () => {
      cancelled = true;
      c.close();
    };
  }, [key]);

  if (!key || error) return <Setup error={error} onKey={(k) => { setError(""); setKey(k); }} />;
  if (!client) return <p className="p-10 text-center text-sm text-muted">Raising the flags…</p>;
  return (
    <FlagpoleProvider client={client}>
      <App />
    </FlagpoleProvider>
  );
}

function Setup({ error, onKey }: { error: string; onKey: (key: string) => void }) {
  const [value, setValue] = useState("");
  return (
    <div className="mx-auto max-w-lg px-6 py-20">
      <h1 className="display text-4xl font-semibold">Harbour Supply Co.</h1>
      <p className="mt-3 text-ink-2">This demo shop reads its flags from Flagpole with an SDK key for web-app's development environment.</p>
      {error && <p className="mt-4 rounded-lg bg-rust/10 px-3 py-2 text-sm text-rust">{error}. Is the API running at {url}, and is the key right?</p>}
      <ul className="mt-6 list-disc space-y-1 pl-5 text-sm text-ink-2">
        <li><code className="font-mono">make seed</code> writes one to <code className="font-mono">demo/.env.local</code>; restart <code className="font-mono">make demo</code> to pick it up.</li>
        <li>Or create one in the dashboard: Web app → Settings → Development → Create key, and paste it here.</li>
      </ul>
      <form
        className="mt-6 flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          try {
            localStorage.setItem("harbour-sdk-key", value.trim());
          } catch {
            /* private mode */
          }
          onKey(value.trim());
        }}
      >
        <input value={value} onChange={(e) => setValue(e.target.value)} placeholder="fp_dev_…" className="flex-1 rounded-lg border border-rule bg-card px-3 py-2 font-mono text-sm" required />
        <button className="rounded-lg bg-ink px-4 py-2 text-sm font-semibold text-white">Use key</button>
      </form>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <Root />
  </StrictMode>,
);
