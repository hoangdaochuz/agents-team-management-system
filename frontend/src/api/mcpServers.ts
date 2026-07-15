import { request } from "./client";
import type { ID, McpServer } from "./types";

export function listMcpServers() {
  return request<McpServer[]>("/mcp-servers");
}

export function getMcpServer(id: ID) {
  return request<McpServer>(`/mcp-servers/${id}`);
}

export function createMcpServer(input: {
  name: string;
  command: string;
  args?: string[];
  env?: Record<string, string>;
}) {
  return request<McpServer>("/mcp-servers", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateMcpServer(id: ID, input: Partial<McpServer>) {
  return request<McpServer>(`/mcp-servers/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function deleteMcpServer(id: ID) {
  return request<void>(`/mcp-servers/${id}`, { method: "DELETE" });
}

export const mcpServers = {
  listMcpServers,
  getMcpServer,
  createMcpServer,
  updateMcpServer,
  deleteMcpServer,
};
