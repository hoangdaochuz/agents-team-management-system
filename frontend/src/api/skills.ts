import { request } from "./client";
import type { ID, Skill } from "./types";

export function listSkills() {
  return request<Skill[]>("/skills");
}

export function getSkill(id: ID) {
  return request<Skill>(`/skills/${id}`);
}

export function createSkill(input: { name: string; description?: string; body_md: string }) {
  return request<Skill>("/skills", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateSkill(id: ID, input: Partial<Skill>) {
  return request<Skill>(`/skills/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function deleteSkill(id: ID) {
  return request<void>(`/skills/${id}`, { method: "DELETE" });
}

export const skills = { listSkills, getSkill, createSkill, updateSkill, deleteSkill };
