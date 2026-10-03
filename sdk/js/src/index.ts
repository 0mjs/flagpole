// Flagpole's JavaScript SDK, for browsers and anything else with fetch.
//
// The client asks the API to evaluate every flag for one user, keeps the
// answers, and re-asks whenever the live stream says a flag changed. Reading a
// flag is synchronous and records an exposure, which the client reports in
// batches.
//
//   const client = new FlagpoleClient({ url, key, context: { key: "user-42" } });
//   await client.start();
//   client.variation("new-checkout", false);

export type Reason = "off" | "rule" | "rollout" | "default" | "not_found";

export interface Context {
  /** Identifies the user; percentage rollouts give the same key the same variant. */
  key: string;
  /** Anything rules can target, such as country or plan. */
  attributes?: Record<string, unknown>;
}

export interface Result {
  variant: string;
  value: unknown;
  reason: Reason;
  /** The index of the rule that matched. */
  rule?: number | null;
}

export type Status = "connecting" | "live" | "reconnecting" | "closed";

export interface Options {
  /** The Flagpole API, such as http://localhost:8080. */
  url: string;
  /** An SDK key for one environment. */
  key: string;
  context: Context;
  /** Report exposures. On by default. */
  exposures?: boolean;
  /** How often to send exposures, in milliseconds. */
  flushInterval?: number;
}

/** What changed: each flag whose variant, or the reason for it, is different from before. */
export interface Change {
  flags: string[];
  version: string;
}

type Events = {
  change: (change: Change) => void;
  status: (status: Status) => void;
  error: (error: Error) => void;
};

interface Exposure {
  flag: string;
  variant: string;
  at: string;
}

export class FlagpoleClient {
  private opts: Required<Options>;
  private results: Record<string, Result> = {};
  private snapshot = "";
  private listeners: { [K in keyof Events]: Set<Events[K]> } = { change: new Set(), status: new Set(), error: new Set() };
  private pending: Exposure[] = [];
  private seen = new Set<string>();
  private stream?: AbortController;
  private flushTimer?: ReturnType<typeof setInterval>;
  private retry?: ReturnType<typeof setTimeout>;
  private evaluating?: Promise<void>;
  private again = false;
  private _status: Status = "closed";

  constructor(options: Options) {
    this.opts = { exposures: true, flushInterval: 5000, ...options, url: options.url.replace(/\/$/, "") };
  }

  /** Evaluates every flag, then listens for changes. Resolves once flags are ready. */
  async start(): Promise<void> {
    this.setStatus("connecting");
    await this.evaluate();
    this.connect(0);
    if (this.opts.exposures) {
      this.flushTimer = setInterval(() => void this.flush(), this.opts.flushInterval);
      if (typeof document !== "undefined") document.addEventListener("visibilitychange", this.onHide);
    }
  }

  /** Stops listening and sends any exposures still waiting. */
  close(): void {
    this.stream?.abort();
    clearTimeout(this.retry);
    clearInterval(this.flushTimer);
    if (typeof document !== "undefined") document.removeEventListener("visibilitychange", this.onHide);
    void this.flush(true);
    this.setStatus("closed");
  }

  /** Switches to another user and re-evaluates every flag for them. */
  async identify(context: Context): Promise<void> {
    void this.flush();
    this.opts.context = context;
    this.seen.clear();
    await this.evaluate();
  }

  get context(): Context {
    return this.opts.context;
  }

  get status(): Status {
    return this._status;
  }

  /** The snapshot version these results came from. */
  get version(): string {
    return this.snapshot;
  }

  /** A flag's value for the current user, or the fallback if it doesn't exist. Records an exposure. */
  variation<T>(flag: string, fallback: T): T {
    const r = this.results[flag];
    if (!r || r.reason === "not_found") return fallback;
    this.expose(flag, r.variant);
    return r.value as T;
  }

  /** The full result for a flag, without recording an exposure. */
  result(flag: string): Result | undefined {
    return this.results[flag];
  }

  /** Every flag's result, without recording exposures. */
  all(): Readonly<Record<string, Result>> {
    return this.results;
  }

  on<K extends keyof Events>(event: K, fn: Events[K]): () => void {
    this.listeners[event].add(fn);
    return () => this.listeners[event].delete(fn);
  }

  private emit<K extends keyof Events>(event: K, ...args: Parameters<Events[K]>): void {
    for (const fn of this.listeners[event]) (fn as (...a: Parameters<Events[K]>) => void)(...args);
  }

  private setStatus(s: Status): void {
    if (s === this._status) return;
    this._status = s;
    this.emit("status", s);
  }

  private headers(json = false): HeadersInit {
    return { Authorization: `Bearer ${this.opts.key}`, ...(json ? { "Content-Type": "application/json" } : {}) };
  }

  // One evaluation at a time; a change that arrives during one runs another after it.
  private evaluate(): Promise<void> {
    if (this.evaluating) {
      this.again = true;
      return this.evaluating;
    }
    // evaluating is cleared in the same step as the last check of again, so
    // a call can't join an evaluation that has already finished.
    this.evaluating = (async () => {
      try {
        do {
          this.again = false;
          const res = await fetch(`${this.opts.url}/sdk/v1/evaluate`, {
            method: "POST",
            headers: this.headers(true),
            body: JSON.stringify({ context: this.opts.context }),
          });
          if (!res.ok) throw new Error(`flagpole: evaluate failed with ${res.status}`);
          const body = (await res.json()) as { version: string; flags: Record<string, Result> | null };
          const next = body.flags ?? {};
          const same = (a?: Result, b?: Result) => a?.variant === b?.variant && a?.reason === b?.reason && a?.rule === b?.rule;
          const changed = Object.keys({ ...this.results, ...next }).filter((k) => !same(this.results[k], next[k]));
          this.results = next;
          this.snapshot = body.version;
          if (changed.length) this.emit("change", { flags: changed, version: body.version });
        } while (this.again);
      } finally {
        this.evaluating = undefined;
      }
    })();
    return this.evaluating;
  }

  // The stream needs the Authorization header, which EventSource can't send,
  // so it's read with fetch.
  private connect(attempt: number): void {
    const ctrl = new AbortController();
    this.stream = ctrl;
    const reconnect = () => {
      if (ctrl.signal.aborted || this._status === "closed") return;
      this.setStatus("reconnecting");
      const delay = Math.min(30_000, 1000 * 2 ** attempt) * (0.8 + Math.random() * 0.4);
      this.retry = setTimeout(() => this.connect(attempt + 1), delay);
    };
    void (async () => {
      try {
        const res = await fetch(`${this.opts.url}/sdk/v1/stream`, { headers: this.headers(), signal: ctrl.signal });
        if (res.status === 401) {
          this.emit("error", new Error("flagpole: the SDK key was refused"));
          this.setStatus("closed");
          return;
        }
        if (!res.ok || !res.body) throw new Error(`flagpole: stream failed with ${res.status}`);
        for await (const event of readEvents(res.body)) {
          if (event.name === "ready") {
            attempt = 0;
            this.setStatus("live");
            // Catch up on anything that changed while disconnected.
            if (event.data?.version !== this.snapshot) await this.evaluate();
          } else if (event.name === "changed" && event.data?.version !== this.snapshot) {
            await this.evaluate();
          }
        }
        reconnect();
      } catch (err) {
        if (ctrl.signal.aborted) return;
        this.emit("error", err instanceof Error ? err : new Error(String(err)));
        reconnect();
      }
    })();
  }

  // One exposure per user, flag and variant: enough to count who saw what,
  // without one per render.
  private expose(flag: string, variant: string): void {
    if (!this.opts.exposures) return;
    const id = `${this.opts.context.key}\u0000${flag}\u0000${variant}`;
    if (this.seen.has(id)) return;
    this.seen.add(id);
    this.pending.push({ flag, variant, at: new Date().toISOString() });
  }

  private onHide = () => {
    if (document.visibilityState === "hidden") void this.flush(true);
  };

  private async flush(leaving = false): Promise<void> {
    if (!this.pending.length) return;
    const batch = this.pending.splice(0, 1000);
    try {
      const res = await fetch(`${this.opts.url}/sdk/v1/exposures`, {
        method: "POST",
        headers: this.headers(true),
        body: JSON.stringify({ exposures: batch }),
        keepalive: leaving,
      });
      if (!res.ok && res.status >= 500) this.pending.unshift(...batch);
    } catch {
      this.pending.unshift(...batch);
    }
  }
}

interface ServerEvent {
  name: string;
  data?: { version?: string; flag?: string };
}

/** Parses a server-sent event stream. */
export async function* readEvents(body: ReadableStream<Uint8Array>): AsyncGenerator<ServerEvent> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) return;
    buffer += decoder.decode(value, { stream: true });
    let end: number;
    while ((end = buffer.search(/\r?\n\r?\n/)) >= 0) {
      const block = buffer.slice(0, end);
      buffer = buffer.slice(end).replace(/^\r?\n\r?\n/, "");
      let name = "message";
      const data: string[] = [];
      for (const line of block.split(/\r?\n/)) {
        if (line.startsWith("event:")) name = line.slice(6).trim();
        else if (line.startsWith("data:")) data.push(line.slice(5).replace(/^ /, ""));
      }
      let parsed: ServerEvent["data"];
      try {
        parsed = data.length ? JSON.parse(data.join("\n")) : undefined;
      } catch {
        parsed = undefined;
      }
      yield { name, data: parsed };
    }
  }
}
