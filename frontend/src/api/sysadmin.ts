import { request } from "./client";
import type { AuditEntry, FeatureFlag, ID, Organization, SignupRequest, SystemHealth, SystemKpis } from "./types";

// ── Organizations ─────────────────────────────────────────────────
export function listOrgs() {
  return request<Organization[]>("/sysadmin/orgs");
}

export function suspendOrg(id: ID) {
  return request<Organization>(`/sysadmin/orgs/${id}/suspend`, { method: "POST" });
}

export function restoreOrg(id: ID) {
  return request<Organization>(`/sysadmin/orgs/${id}/restore`, { method: "POST" });
}

export interface CreateOrgInput {
  name: string;
  plan: Organization["plan"];
}

export function createOrg(input: CreateOrgInput) {
  return request<Organization>("/sysadmin/orgs", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

// ── Cross-org sign-up requests ────────────────────────────────────
export function listSignupRequests() {
  return request<SignupRequest[]>("/sysadmin/requests");
}

export function approveSignup(id: ID) {
  return request<void>(`/sysadmin/requests/${id}/approve`, { method: "POST" });
}

// ── Feature flags ─────────────────────────────────────────────────
export function listFeatureFlags() {
  return request<FeatureFlag[]>("/sysadmin/flags");
}

export function toggleFeatureFlag(key: string, enabled: boolean) {
  return request<FeatureFlag>(`/sysadmin/flags/${encodeURIComponent(key)}`, {
    method: "PATCH",
    body: JSON.stringify({ enabled }),
  });
}

// ── System health / audit / ops ───────────────────────────────────
export function kpis() {
  return request<SystemKpis>("/sysadmin/kpis");
}

export function systemHealth() {
  return request<SystemHealth>("/sysadmin/health");
}

export function systemAudit() {
  return request<AuditEntry[]>("/sysadmin/audit");
}

export function runMaintenance() {
  return request<{ ok: true }>("/sysadmin/maintenance", { method: "POST" });
}

export const sysadmin = {
  listOrgs,
  suspendOrg,
  restoreOrg,
  createOrg,
  listSignupRequests,
  approveSignup,
  listFeatureFlags,
  toggleFeatureFlag,
  kpis,
  systemHealth,
  systemAudit,
  runMaintenance,
};
