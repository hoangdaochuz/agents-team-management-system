import { request, qs } from "./client";
import type { AuditEntry, ID } from "./types";

export function list(workspaceId: ID, filter?: { kind?: string }) {
  return request<AuditEntry[]>(`/workspaces/${workspaceId}/audit${qs(filter)}`);
}

/** Triggers a backend export (e.g. emailed CSV / signed URL). */
export function exportLog(workspaceId: ID) {
  return request<{ ok: true }>(`/workspaces/${workspaceId}/audit/export`, {
    method: "POST",
  });
}

export const audit = { list, exportLog };
