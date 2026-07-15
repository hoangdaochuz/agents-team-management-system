import { request } from "./client";
import type { Artifact, Finding, ID, Run, Step } from "./types";

export function listRuns(taskId: ID) {
  return request<Run[]>(`/tasks/${taskId}/runs`);
}

export function listRunSteps(runId: ID) {
  return request<Step[]>(`/runs/${runId}/steps`);
}

export function listRunFindings(runId: ID) {
  return request<Finding[]>(`/runs/${runId}/findings`);
}

export function listTaskArtifacts(taskId: ID) {
  return request<Artifact[]>(`/tasks/${taskId}/artifacts`);
}

export const runs = { listRuns, listRunSteps, listRunFindings, listTaskArtifacts };
