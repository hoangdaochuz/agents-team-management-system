import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { workspaces } from "../api/client";
import type { Workspace } from "../api/types";
import { useAuth } from "../store/auth";
import { AsyncBoundary, Button } from "../components/ui";
import { Icon } from "../lib/icons";
import { NewWorkspaceModal } from "../components/workspaces/NewWorkspaceModal";

export function WorkspacesPage() {
  const navigate = useNavigate();
  const setActiveWorkspace = useAuth((s) => s.setActiveWorkspace);
  const activeId = useAuth((s) => s.activeWorkspace?.id);
  const [creating, setCreating] = useState(false);

  const { data, isLoading, isError, error } = useQuery<Workspace[]>({
    queryKey: ["workspaces"],
    queryFn: () => workspaces.list(),
  });

  const select = (ws: Workspace) => {
    setActiveWorkspace(ws.id);
    navigate("/dashboard");
  };

  return (
    <>
      <div className="page-head">
        <div>
          <h1 className="page-title">Workspaces</h1>
          <div className="page-sub">{(data ?? []).length} workspaces · switch context anytime</div>
        </div>
        <Button variant="primary" onClick={() => setCreating(true)}>
          + New workspace
        </Button>
      </div>

      <div className="scope-banner">
        <Icon name="building" size={18} />
        <div style={{ flex: 1, fontSize: "var(--text-sm)" }}>
          You belong to {(data ?? []).length} workspace(s). Each workspace isolates its repo, agents, and members.
        </div>
      </div>

      <AsyncBoundary
        isLoading={isLoading}
        isError={isError}
        error={error}
        data={data}
        isEmpty={(d: Workspace[]) => d.length === 0}
        emptyTitle="No workspaces yet"
        emptyHint="Create your first workspace to start assigning agent work."
      >
        {(list: Workspace[]) => (
          <div className="grid ws-grid">
            {list.map((ws) => {
              const glyph = ws.glyph ?? ws.name.slice(0, 2).toUpperCase();
              const isActive = ws.id === activeId;
              return (
                <button
                  key={ws.id}
                  className="ws-card card"
                  onClick={() => select(ws)}
                  style={{ textAlign: "left", cursor: "pointer", border: isActive ? "1px solid var(--accent)" : undefined }}
                >
                  <div className="ws-card-top">
                    <span className="ws-glyph">{glyph}</span>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontWeight: 600 }}>{ws.name}</div>
                      {ws.repo_source && (
                        <div className="muted mono" style={{ fontSize: 12 }}>
                          {ws.repo_source}
                        </div>
                      )}
                    </div>
                    {isActive && <span className="badge badge-accent">Current</span>}
                  </div>
                  {ws.description && <p className="muted" style={{ fontSize: "var(--text-sm)" }}>{ws.description}</p>}
                  <div className="ws-stats">
                    <span>
                      <b className="ws-stat-num">{ws.agent_count ?? 0}</b>{" "}
                      <span className="ws-stat-lbl">agents</span>
                    </span>
                    <span>
                      <b className="ws-stat-num">{ws.open_task_count ?? 0}</b>{" "}
                      <span className="ws-stat-lbl">open tasks</span>
                    </span>
                    <span>
                      <span className={`role-pill role-${ws.role}`}>{ws.role}</span>
                    </span>
                  </div>
                </button>
              );
            })}

          </div>
        )}
      </AsyncBoundary>

      <section className="card" style={{ marginTop: "var(--space-6)" }}>
        <div className="card-head">
          <h2 className="card-title">How workspace isolation works</h2>
        </div>
        <div className="grid" style={{ gridTemplateColumns: "repeat(3, 1fr)", gap: "var(--space-4)" }}>
          <div>
            <div style={{ fontWeight: 600, marginBottom: 4 }}>Separate repos</div>
            <p className="muted" style={{ fontSize: "var(--text-sm)" }}>
              Each workspace maps to one repository. Agents never see files outside it.
            </p>
          </div>
          <div>
            <div style={{ fontWeight: 600, marginBottom: 4 }}>Scoped agents &amp; skills</div>
            <p className="muted" style={{ fontSize: "var(--text-sm)" }}>
              Agents, skills, and MCP servers belong to a workspace and don't leak across.
            </p>
          </div>
          <div>
            <div style={{ fontWeight: 600, marginBottom: 4 }}>Per-workspace members</div>
            <p className="muted" style={{ fontSize: "var(--text-sm)" }}>
              Membership and roles are scoped per workspace, not global.
            </p>
          </div>
        </div>
      </section>

      <NewWorkspaceModal open={creating} onClose={() => setCreating(false)} />
    </>
  );
}
