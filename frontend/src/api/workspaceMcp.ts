import { request } from "./client";
import type { ID, McpConnection } from "./types";

export function list(workspaceId: ID) {
  return request<McpConnection[]>(`/workspaces/${workspaceId}/mcp`);
}

export function reconnect(workspaceId: ID, id: ID) {
  return request<McpConnection>(`/workspaces/${workspaceId}/mcp/${id}/reconnect`, {
    method: "POST",
  });
}

export const workspaceMcp = { list, reconnect };
