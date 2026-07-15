import { request } from "./client";
import type { Feedback, ID } from "./types";

export function listFeedback(taskId: ID) {
  return request<Feedback[]>(`/tasks/${taskId}/feedback`);
}

export function addFeedback(taskId: ID, body: string) {
  return request<Feedback>(`/tasks/${taskId}/feedback`, {
    method: "POST",
    body: JSON.stringify({ body }),
  });
}

export const feedback = { listFeedback, addFeedback };
