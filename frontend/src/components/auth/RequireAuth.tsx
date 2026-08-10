import { type ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import type { Role } from "../../api/types";
import { useAuth } from "../../store/auth";
import { NoAccess } from "./NoAccess";

/** Redirect unauthenticated users to /login, preserving where they were headed. */
export function RequireAuth({ children }: { children: ReactNode }) {
  const status = useAuth((s) => s.status);
  const location = useLocation();

  if (status === "loading") {
    return (
      <div style={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <div className="muted">Loading…</div>
      </div>
    );
  }
  if (status !== "authenticated") {
    return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />;
  }
  return <>{children}</>;
}

/** Show a "no access" state when the active role is insufficient (no redirect). */
export function RequireRole({ role, children }: { role: Role; children: ReactNode }) {
  const hasRole = useAuth((s) => s.hasRole);
  if (!hasRole(role)) return <NoAccess role={role} />;
  return <>{children}</>;
}

/**
 * Gate the system console on the *superadmin* flag — a workspace role
 * (owner/admin) is NOT sufficient. Spec: "Access SHALL require the superadmin role."
 */
export function RequireSuperadmin({ children }: { children: ReactNode }) {
  const isSuperadmin = useAuth((s) => Boolean(s.user?.is_superadmin));
  if (!isSuperadmin) return <NoAccess />;
  return <>{children}</>;
}
