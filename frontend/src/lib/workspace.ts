import { useAuth } from "../store/auth";

/**
 * Read the active workspace id from the session store (hook, for components).
 * Used in query keys so cached data is partitioned per workspace and switching
 * the active workspace triggers a refetch (design D3).
 */
export function useActiveWorkspaceId(): string | undefined {
  return useAuth((s) => s.activeWorkspace?.id);
}
