import { request } from "./client";
import type { Agent, ID } from "./types";

export function listAgents() {
  return request<Agent[]>("/agents");
}

export function getAgent(id: ID) {
  return request<Agent>(`/agents/${id}`);
}

export function createAgent(input: {
  name: string;
  role: string;
  system_prompt?: string;
  default_model?: string;
  allowed_tools?: string[];
}) {
  return request<Agent>("/agents", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateAgent(id: ID, input: Partial<Agent>) {
  return request<Agent>(`/agents/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function deleteAgent(id: ID) {
  return request<void>(`/agents/${id}`, { method: "DELETE" });
}

export function attachSkill(agentId: ID, skillId: ID) {
  return request<void>(`/agents/${agentId}/skills`, {
    method: "POST",
    body: JSON.stringify({ skill_id: skillId }),
  });
}

export function detachSkill(agentId: ID, skillId: ID) {
  return request<void>(`/agents/${agentId}/skills/${skillId}`, { method: "DELETE" });
}

export function attachMcp(agentId: ID, mcpId: ID) {
  return request<void>(`/agents/${agentId}/mcps`, {
    method: "POST",
    body: JSON.stringify({ mcp_server_id: mcpId }),
  });
}

export function detachMcp(agentId: ID, mcpId: ID) {
  return request<void>(`/agents/${agentId}/mcps/${mcpId}`, { method: "DELETE" });
}

export const agents = {
  listAgents,
  getAgent,
  createAgent,
  updateAgent,
  deleteAgent,
  attachSkill,
  detachSkill,
  attachMcp,
  detachMcp,
};
