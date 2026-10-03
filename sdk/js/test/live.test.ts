// Runs the SDK against a running Flagpole (make up, api, worker, seed): it
// changes a flag through the API, as someone in the dashboard would, and
// checks the client hears about it.
//
//   FLAGPOLE_KEY=fp_dev_… node --test test/
import { after, before, test } from "node:test";
import assert from "node:assert/strict";
import { FlagpoleClient, type Change } from "../src/index.ts";

const url = process.env.FLAGPOLE_URL ?? "http://localhost:8080";
const key = process.env.FLAGPOLE_KEY ?? "";
const password = process.env.FLAGPOLE_PASSWORD ?? "flagpole-demo-password";
const flag = "maintenance-banner";
const path = `/api/v1/projects/web-app/flags/${flag}/environments/development`;

// A minimal dashboard session: cookies and the CSRF header.
let cookies: Record<string, string> = {};
async function api(method: string, p: string, body?: unknown) {
  const res = await fetch(url + p, {
    method,
    headers: {
      Cookie: Object.entries(cookies).map(([k, v]) => `${k}=${v}`).join("; "),
      "X-CSRF-Token": cookies.flagpole_csrf ?? "",
      ...(body ? { "Content-Type": "application/json" } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  for (const c of res.headers.getSetCookie()) {
    const [pair] = c.split(";");
    const i = pair.indexOf("=");
    cookies = { ...cookies, [pair.slice(0, i)]: pair.slice(i + 1) };
  }
  return res;
}

function nextChange(client: FlagpoleClient, ms = 5000): Promise<Change> {
  return new Promise((resolve, reject) => {
    const t = setTimeout(() => reject(new Error(`no change within ${ms}ms`)), ms);
    const off = client.on("change", (c) => {
      clearTimeout(t);
      off();
      resolve(c);
    });
  });
}

let client: FlagpoleClient;
let original: { enabled: boolean; version: number };

before(async () => {
  assert.ok(key, "set FLAGPOLE_KEY to an SDK key for web-app development");
  await api("GET", "/api/v1/auth/me");
  assert.equal((await api("POST", "/api/v1/auth/login", { email: "grace@example.com", password })).status, 200);
  const flagRes = await api("GET", `/api/v1/projects/web-app/flags/${flag}`);
  const cfg = ((await flagRes.json()) as { environments: { environment: string; enabled: boolean; version: number }[] }).environments.find((e) => e.environment === "development")!;
  original = { enabled: cfg.enabled, version: cfg.version };
  client = new FlagpoleClient({ url, key, context: { key: "sdk-test", attributes: { country: "uk" } }, exposures: false });
  await client.start();
});

after(async () => {
  client?.close();
  if (original) {
    const res = await api("GET", `/api/v1/projects/web-app/flags/${flag}`);
    const cfg = ((await res.json()) as { environments: { environment: string; version: number }[] }).environments.find((e) => e.environment === "development")!;
    await api("PUT", path, { enabled: original.enabled, default_variant: "on", off_variant: "off", base_version: cfg.version });
  }
});

test("start evaluates every flag", () => {
  assert.equal(client.result(flag)?.reason, original.enabled ? "default" : "off");
  assert.equal(client.variation("does-not-exist", "fallback"), "fallback");
});

test("the stream goes live", async () => {
  for (let i = 0; i < 40 && client.status !== "live"; i++) await new Promise((r) => setTimeout(r, 50));
  assert.equal(client.status, "live");
});

test("a change in the dashboard reaches the client", async () => {
  const heard = nextChange(client);
  const started = Date.now();
  const res = await api("PUT", path, { enabled: !original.enabled, default_variant: "on", off_variant: "off", base_version: original.version });
  assert.equal(res.status, 200);
  const change = await heard;
  assert.deepEqual(change.flags, [flag]);
  assert.equal(client.variation(flag, original.enabled), !original.enabled);
  console.log(`      heard in ${Date.now() - started} ms`);
});

test("a rule targets the user the client identifies as", async () => {
  const cfg = { enabled: true, default_variant: "off", off_variant: "off", rules: [{ conditions: [{ attribute: "country", operator: "in", values: ["uk"] }], variant: "on" }] };
  const flagRes = await api("GET", `/api/v1/projects/web-app/flags/${flag}`);
  const version = ((await flagRes.json()) as { environments: { environment: string; version: number }[] }).environments.find((e) => e.environment === "development")!.version;
  const heard = nextChange(client);
  assert.equal((await api("PUT", path, { ...cfg, base_version: version })).status, 200);
  await heard;
  assert.equal(client.variation(flag, false), true, "uk gets on by the rule");
  assert.equal(client.result(flag)?.reason, "rule");

  await client.identify({ key: "sdk-test-2", attributes: { country: "fr" } });
  assert.equal(client.variation(flag, true), false, `fr gets the default, got ${JSON.stringify(client.result(flag))} for ${JSON.stringify(client.context)}`);
  assert.equal(client.result(flag)?.reason, "default");
});
