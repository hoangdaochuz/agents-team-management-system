import { request } from "./client";
import type { ID, Plugin } from "./types";

export function list(workspaceId: ID) {
  return request<Plugin[]>(`/workspaces/${workspaceId}/plugins`);
}

export function setEnabled(workspaceId: ID, id: ID, enabled: boolean) {
  return request<Plugin>(`/workspaces/${workspaceId}/plugins/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ enabled }),
  });
}

export const plugins = { list, setEnabled };
