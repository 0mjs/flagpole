import createClient, { type Middleware } from "openapi-fetch";
import type { components, paths } from "./schema";

export type Schemas = components["schemas"];

// Every change carries the CSRF token from the flagpole_csrf cookie, which
// the API checks against the cookie itself.
function csrfToken(): string {
  return document.cookie.split("; ").find((c) => c.startsWith("flagpole_csrf="))?.split("=")[1] ?? "";
}

const csrf: Middleware = {
  onRequest({ request }) {
    if (!["GET", "HEAD", "OPTIONS"].includes(request.method)) {
      request.headers.set("X-CSRF-Token", csrfToken());
    }
    return request;
  },
};

export const api = createClient<paths>({ baseUrl: "/", credentials: "same-origin" });
api.use(csrf);

/** An RFC 9457 problem, as the API sends errors. */
export class Problem extends Error {
  constructor(
    public status: number,
    public title: string,
    public detail: string | undefined,
    public fields: Record<string, string> = {},
  ) {
    super(detail ?? title);
  }
}

/** Unwraps an openapi-fetch result, throwing the API's problem on failure. */
export async function unwrap<T>(p: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
  const { data, error, response } = await p;
  if (!response.ok) {
    const e = (error ?? {}) as { title?: string; detail?: string; errors?: Record<string, string> };
    throw new Problem(response.status, e.title ?? response.statusText, e.detail, e.errors ?? {});
  }
  return data as T;
}
