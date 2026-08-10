import { useEffect, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { auth } from "../../api/client";
import type { Session } from "../../api/types";
import { useAuth } from "../../store/auth";

/**
 * Boots the session before rendering children.
 *
 * - Calls GET /api/auth/me on mount.
 * - On success, hydrates the session store.
 * - On failure while the backend is unimplemented, synthesizes a single-operator
 *   "dev-fallback" session so the rest of the SPA stays navigable (clearly badged).
 * - In a production build with no session, leaves status unauthenticated so
 *   <RequireAuth> can redirect to /login.
 */
export function AppGate({ children }: { children: ReactNode }) {
  const hydrate = useAuth((s) => s.hydrate);
  const status = useAuth((s) => s.status);
  const devFallback = useAuth((s) => s.devFallback);

  const { isSuccess, data, isError } = useQuery<Session>({
    queryKey: ["auth", "me"],
    queryFn: () => auth.me(),
    staleTime: Infinity,
  });

  useEffect(() => {
    if (isSuccess && data && status !== "authenticated") {
      hydrate(data);
      return;
    }
    if (isError && status === "loading") {
      if (import.meta.env.DEV || import.meta.env.VITE_DEV_NOAUTH) {
        hydrate(devFallbackSession(), { devFallback: true });
      } else {
        useAuth.setState({ status: "unauthenticated" });
      }
    }
  }, [isSuccess, data, isError, status, hydrate]);

  if (status === "loading") {
    return (
      <div style={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <div className="muted">Loading…</div>
      </div>
    );
  }

  return (
    <>
      {devFallback && (
        <div
          style={{
            position: "fixed",
            bottom: 12,
            left: 12,
            zIndex: 1000,
            fontSize: 11,
            fontWeight: 600,
            padding: "4px 10px",
            borderRadius: 999,
            background: "var(--warn, #f0a020)",
            color: "#1a1a1a",
          }}
        >
          Dev fallback — no backend
        </div>
      )}
      {children}
    </>
  );
}

/** A synthetic single-operator session used when the auth backend is absent. */
function devFallbackSession(): Session {
  const ws = {
    id: "ws-dev",
    name: "Agent Ops",
    repo_source: "github.com/hoangdaochuz/agents-team-management-system",
    glyph: "AO",
    description: "Dev workspace (synthetic)",
    agent_count: 0,
    open_task_count: 0,
    role: "owner" as const,
    created_at: new Date(0).toISOString(),
  };
  return {
    user: {
      id: "user-dev",
      name: "Dang Anh",
      email: "dang@agentops.dev",
      role: "owner",
      is_superadmin: true,
      created_at: new Date(0).toISOString(),
    },
    workspaces: [ws],
    active_workspace_id: ws.id,
  };
}
