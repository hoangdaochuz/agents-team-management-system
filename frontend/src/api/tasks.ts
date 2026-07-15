import { request, qs } from "./client";
import type { ID, Run, Task, TaskStatus, Priority, TaskType } from "./types";

export interface TaskQuery {
  project_id?: ID;
  status?: TaskStatus;
  type?: TaskType;
  priority?: Priority;
  assignee?: ID;
  label?: string;
  q?: string;
}

export function listTasks(query?: TaskQuery) {
  return request<Task[]>(`/tasks${qs(query)}`);
}

export function getTask(id: ID) {
  return request<Task>(`/tasks/${id}`);
}

export function createTask(input: {
  project_id: ID;
  title: string;
  prompt: string;
  description?: string;
  type?: TaskType;
  priority?: Priority;
  labels?: string[];
  points?: number;
  agent_id?: ID;
  due_at?: string;
}) {
  return request<Task>("/tasks", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateTask(id: ID, input: Partial<Task>) {
  return request<Task>(`/tasks/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

/** Move a task between kanban columns. */
export function patchStatus(id: ID, status: TaskStatus) {
  return request<Task>(`/tasks/${id}/status`, {
    method: "PATCH",
    body: JSON.stringify({ status }),
  });
}

export function deleteTask(id: ID) {
  return request<void>(`/tasks/${id}`, { method: "DELETE" });
}

/** Re-run the implementer with the latest feedback. */
export function rerunTask(id: ID) {
  return request<Run>(`/tasks/${id}/re-run`, { method: "POST" });
}

/** Stop a running task. */
export function stopTask(id: ID) {
  return request<Task>(`/tasks/${id}/stop`, { method: "POST" });
}

/** Create a GitHub PR from the task branch. */
export function openPr(id: ID) {
  return request<{ url: string }>(`/tasks/${id}/open-pr`, { method: "POST" });
}

export const tasks = {
  listTasks,
  getTask,
  createTask,
  updateTask,
  patchStatus,
  deleteTask,
  rerunTask,
  stopTask,
  openPr,
};
