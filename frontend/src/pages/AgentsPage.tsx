import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { agents } from "../api/client";
import type { Agent } from "../api/types";
import { AsyncBoundary, Button } from "../components/ui";
import { AgentCard } from "../components/agents/AgentCard";
import { AgentFormModal } from "../components/agents/AgentFormModal";

export function AgentsPage() {
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Agent | undefined>(undefined);

  const { data, isLoading, isError, error } = useQuery<Agent[]>({
    queryKey: ["agents"],
    queryFn: () => agents.listAgents(),
  });

  const running = (data ?? []).filter((a) => a.status === "running").length;

  return (
    <>
      <div className="page-head">
        <div>
          <h1 className="page-title">Agents</h1>
          <div className="page-sub">
            {(data ?? []).length} agents in the workspace · {running} running
          </div>
        </div>
        <Button variant="primary" onClick={() => setCreating(true)}>
          + New agent
        </Button>
      </div>

      <AsyncBoundary
        isLoading={isLoading}
        isError={isError}
        error={error}
        data={data}
        isEmpty={(d: Agent[]) => d.length === 0}
        emptyTitle="No agents yet"
        emptyHint="Create your first agent to start assigning work."
      >
        {(list: Agent[]) => (
          <div className="grid" style={{ gridTemplateColumns: "repeat(3, 1fr)" }}>
            {list.map((a) => (
              <AgentCard key={a.id} agent={a} onEdit={() => setEditing(a)} />
            ))}
          </div>
        )}
      </AsyncBoundary>

      <AgentFormModal
        open={creating || Boolean(editing)}
        onClose={() => {
          setCreating(false);
          setEditing(undefined);
        }}
        agent={editing}
      />
    </>
  );
}
