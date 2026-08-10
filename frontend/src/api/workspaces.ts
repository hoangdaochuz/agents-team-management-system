import { request } from "./client";
import type { ID, Workspace } from "./types";

export function list() {
  return request<Workspace[]>("/workspaces");
}

export function get(id: ID) {
  return request<Workspace>(`/workspaces/${id}`);
}

export interface CreateWorkspaceInput {
  name: string;
  repo_source: string;
  default_branch?: string;
  role: "owner" | "admin";
}

export function create(input: CreateWorkspaceInput) {
  return request<Workspace>("/workspaces", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export const workspaces = { list, get, create };
