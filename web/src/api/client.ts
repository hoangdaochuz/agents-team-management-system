// Typed API client for the Go backend. All endpoints are under /api and proxied
// to the backend in dev (see vite.config.ts) / same-origin in prod.

export type Health = { status: string };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`${res.status} ${res.statusText} ${body}`.trim());
  }
  // 204 / empty body
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export function fetchHealth(): Promise<Health> {
  return request<Health>("/healthz");
}

// Example shape for later phases:
// export type Task = { id: string; title: string; status: string };
// export const fetchTasks = (projectId: string) => request<Task[]>(`/projects/${projectId}/tasks`);
