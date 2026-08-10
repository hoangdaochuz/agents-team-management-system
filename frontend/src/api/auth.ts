import { request } from "./client";
import type { ID, Role, Session, SsoProvider } from "./types";

export interface LoginInput {
  email: string;
  password: string;
  remember?: boolean;
}

export interface SignupInput {
  full_name: string;
  email: string;
  password: string;
  start_mode: "join" | "create";
  invite_code?: string;
  organization_name?: string;
}

export interface SignupStatus {
  state: "pending" | "approved" | "declined";
  email: string;
  workspace_name?: string;
  admin_name?: string;
}

export function login(input: LoginInput) {
  return request<Session>("/auth/login", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function signup(input: SignupInput) {
  return request<{ request_id: ID }>("/auth/signup", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function me() {
  return request<Session>("/auth/me");
}

export function logout() {
  return request<void>("/auth/logout", { method: "POST" });
}

export function signupStatus() {
  return request<SignupStatus>("/auth/signup-status");
}

export function resendSignupNotification() {
  return request<void>("/auth/signup-status/resend", { method: "POST" });
}

/** Begin an SSO flow. Declared stub until the backend implements it. */
export function ssoBegin(provider: SsoProvider) {
  return request<{ redirect_url: string }>("/auth/sso/begin", {
    method: "POST",
    body: JSON.stringify({ provider }),
  });
}

export type { Role };
