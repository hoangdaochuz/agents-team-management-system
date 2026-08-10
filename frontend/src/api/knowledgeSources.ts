import { request } from "./client";
import type { ID, KnowledgeSource } from "./types";

export function list(workspaceId: ID) {
  return request<KnowledgeSource[]>(`/workspaces/${workspaceId}/knowledge`);
}

export function create(workspaceId: ID, input: Partial<KnowledgeSource>) {
  return request<KnowledgeSource>(`/workspaces/${workspaceId}/knowledge`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export const knowledgeSources = { list, create };
