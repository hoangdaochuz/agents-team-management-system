import { request } from "./client";
import type { ID, Rule } from "./types";

export function list(workspaceId: ID) {
  return request<Rule[]>(`/workspaces/${workspaceId}/rules`);
}

export function setEnabled(workspaceId: ID, id: ID, enabled: boolean) {
  return request<Rule>(`/workspaces/${workspaceId}/rules/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ enabled }),
  });
}

export const rules = { list, setEnabled };
