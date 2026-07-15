import { request } from "./client";
import type { ID, Project, RepoType } from "./types";

export function listProjects() {
  return request<Project[]>("/projects");
}

export function getProject(id: ID) {
  return request<Project>(`/projects/${id}`);
}

export function createProject(input: {
  name: string;
  repo_source: string;
  repo_type: RepoType;
  default_branch?: string;
}) {
  return request<Project>("/projects", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateProject(id: ID, input: Partial<Project>) {
  return request<Project>(`/projects/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function deleteProject(id: ID) {
  return request<void>(`/projects/${id}`, { method: "DELETE" });
}

export const projects = { listProjects, getProject, createProject, updateProject, deleteProject };
