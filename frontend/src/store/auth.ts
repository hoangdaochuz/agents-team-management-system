import { create } from "zustand";
import type { Role, Session, User, Workspace } from "../api/types";

export type AuthStatus = "loading" | "authenticated" | "unauthenticated";

interface PersistedAuth {
  userId?: string;
  activeWorkspaceId?: string;
}

interface AuthState {
  user: User | null;
  activeWorkspace: Workspace | null;
  workspaces: Workspace[];
  status: AuthStatus;
  /** True when the session was synthesized because the backend is unimplemented. */
  devFallback: boolean;
  hydrate: (session: Session, opts?: { devFallback?: boolean }) => void;
  setActiveWorkspace: (id: string) => void;
  login: (session: Session) => void;
  logout: () => void;
  hasRole: (role: Role) => boolean;
}

/** Derive the active workspace from a session, honoring persisted choice. */
function pickActive(session: Session, persistedId?: string): Workspace | null {
  const list = session.workspaces ?? [];
  if (persistedId) {
    const found = list.find((w) => w.id === persistedId);
    if (found) return found;
  }
  if (session.active_workspace_id) {
    const found = list.find((w) => w.id === session.active_workspace_id);
    if (found) return found;
  }
  return list[0] ?? null;
}

const LS_KEY = "agentops.auth.v1";

function readPersisted(): PersistedAuth {
  try {
    const raw = localStorage.getItem(LS_KEY);
    return raw ? (JSON.parse(raw) as PersistedAuth) : {};
  } catch {
    return {};
  }
}

function writePersisted(p: PersistedAuth) {
  try {
    localStorage.setItem(LS_KEY, JSON.stringify(p));
  } catch {
    /* ignore */
  }
}

export const useAuth = create<AuthState>((set, get) => ({
  user: null,
  activeWorkspace: null,
  workspaces: [],
  status: "loading",
  devFallback: false,

  hydrate: (session, opts) => {
    const persisted = readPersisted();
    const active = pickActive(session, persisted.activeWorkspaceId);
    set({
      user: session.user,
      workspaces: session.workspaces ?? [],
      activeWorkspace: active,
      status: "authenticated",
      devFallback: Boolean(opts?.devFallback),
    });
    if (active) writePersisted({ userId: session.user.id, activeWorkspaceId: active.id });
  },

  setActiveWorkspace: (id) => {
    const ws = get().workspaces.find((w) => w.id === id) ?? null;
    set({ activeWorkspace: ws });
    const u = get().user;
    if (u) writePersisted({ userId: u.id, activeWorkspaceId: id });
  },

  login: (session) => get().hydrate(session),

  logout: () => {
    set({ user: null, activeWorkspace: null, workspaces: [], status: "unauthenticated", devFallback: false });
    try {
      localStorage.removeItem(LS_KEY);
    } catch {
      /* ignore */
    }
  },

  hasRole: (role) => {
    const ws = get().activeWorkspace;
    const user = get().user;
    if (!ws || !user) return false;
    // superadmin satisfies every workspace-role check.
    if (user.is_superadmin) return true;
    // owner is a strict superset of admin/member.
    const rank: Record<Role, number> = { member: 1, admin: 2, owner: 3 };
    return rank[ws.role] >= rank[role];
  },
}));

/** Selector helper: the active workspace (or null). */
export function useActiveWorkspace(): Workspace | null {
  return useAuth((s) => s.activeWorkspace);
}
