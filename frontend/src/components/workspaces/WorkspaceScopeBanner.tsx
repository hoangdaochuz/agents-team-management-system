import { Link } from "react-router-dom";
import { useAuth } from "../../store/auth";

/** Shows the active workspace context with a switch affordance. */
export function WorkspaceScopeBanner() {
  const activeWorkspace = useAuth((s) => s.activeWorkspace);
  if (!activeWorkspace) return null;
  const glyph = activeWorkspace.glyph ?? activeWorkspace.name.slice(0, 2).toUpperCase();

  return (
    <div className="scope-banner">
      <span className="ws-glyph">{glyph}</span>
      <div style={{ flex: 1 }}>
        <div style={{ fontWeight: 600 }}>{activeWorkspace.name}</div>
        {activeWorkspace.repo_source && (
          <div className="muted mono" style={{ fontSize: 12 }}>
            {activeWorkspace.repo_source}
          </div>
        )}
      </div>
      <Link to="/workspaces" className="btn btn-ghost btn-sm">
        Switch workspace
      </Link>
    </div>
  );
}
