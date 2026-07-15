// Typed HTTP client. All paths are relative to /api (the Vite dev proxy and the
// Go server both expose the API under /api). Functions here declare the contract
// the backend will implement; until then calls resolve to error states that the
// UI surfaces gracefully.

const BASE = "/api";

/** Build a query string from a params object, skipping null/undefined/"". */
export function qs(params: object | undefined): string {
  if (!params) return "";
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === "") continue;
    sp.append(k, String(v));
  }
  const s = sp.toString();
  return s ? `?${s}` : "";
}

export class ApiError extends Error {
  status: number;
  body: string;
  constructor(status: number, body: string) {
    super(`${status} ${body}`.trim());
    this.status = status;
    this.body = body;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new ApiError(res.status, `${res.statusText} ${body}`.trim());
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export { request };

// Re-export the typed domain modules for ergonomic imports.
export * as projects from "./projects";
export * as tasks from "./tasks";
export * as agents from "./agents";
export * as skills from "./skills";
export * as mcpServers from "./mcpServers";
export * as providerKeys from "./providerKeys";
export * as feedback from "./feedback";
export * as runs from "./runs";

// Health probe — the Phase 0 backend already implements this.
export type Health = { status: string };
export function fetchHealth(): Promise<Health> {
  return request<Health>("/healthz");
}
