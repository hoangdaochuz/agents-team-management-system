import { request } from "./client";
import type { ID, Member, Role } from "./types";

export function list(workspaceId: ID) {
  return request<Member[]>(`/workspaces/${workspaceId}/members`);
}

export function updateRole(workspaceId: ID, memberId: ID, role: Role) {
  return request<Member>(`/workspaces/${workspaceId}/members/${memberId}`, {
    method: "PATCH",
    body: JSON.stringify({ role }),
  });
}

export function remove(workspaceId: ID, memberId: ID) {
  return request<void>(`/workspaces/${workspaceId}/members/${memberId}`, {
    method: "DELETE",
  });
}

export function resendInvite(workspaceId: ID, memberId: ID) {
  return request<void>(`/workspaces/${workspaceId}/members/${memberId}/resend`, {
    method: "POST",
  });
}

export const members = { list, updateRole, remove, resendInvite };
