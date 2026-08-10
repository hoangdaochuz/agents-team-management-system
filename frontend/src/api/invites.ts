import { request } from "./client";
import type { ID, Invite, Role, SignupRequest } from "./types";

/** Pending join requests surfaced to workspace admins for approval. */
export function listPending(workspaceId: ID) {
  return request<SignupRequest[]>(`/workspaces/${workspaceId}/requests`);
}

export function approve(workspaceId: ID, requestId: ID) {
  return request<void>(`/workspaces/${workspaceId}/requests/${requestId}/approve`, {
    method: "POST",
  });
}

export function decline(workspaceId: ID, requestId: ID) {
  return request<void>(`/workspaces/${workspaceId}/requests/${requestId}/decline`, {
    method: "POST",
  });
}

export interface SendInvitesInput {
  emails: string[];
  role: Role;
}

export function send(workspaceId: ID, input: SendInvitesInput) {
  return request<Invite[]>(`/workspaces/${workspaceId}/invites`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export const invites = { listPending, approve, decline, send };
