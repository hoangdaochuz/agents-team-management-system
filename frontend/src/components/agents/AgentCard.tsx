import { useNavigate } from "react-router-dom";
import type { Agent } from "../../api/types";
import { shortId } from "../../lib/format";
import { Avatar, StatusBadge, Progress, Button } from "../ui";

export function AgentCard({ agent, onEdit }: { agent: Agent; onEdit?: () => void }) {
  const navigate = useNavigate();
  const load = typeof agent.load === "number" ? agent.load : 0;
  const isPaused = agent.status === "paused";
  const taskLabel = agent.current_task_id ? shortId(agent.current_task_id) : null;

  return (
    <div className="agent-card">
      <div className="agent-top">
        <Avatar name={agent.name} size="lg" />
        <div style={{ flex: 1 }}>
          <div className="agent-name">{agent.name}</div>
          <div className="agent-role">{agent.role}</div>
        </div>
        <StatusBadge status={agent.status ?? "idle"} />
      </div>

      {agent.capabilities && agent.capabilities.length > 0 && (
        <div className="cap-tags">
          {agent.capabilities.map((c) => (
            <span key={c} className="tag">
              {c}
            </span>
          ))}
        </div>
      )}

      <div>
        <div className="spread" style={{ marginBottom: 5 }}>
          <span className="muted" style={{ fontSize: 12 }}>
            Load today
          </span>
          <span className="mono" style={{ fontSize: 12 }}>
            {load}%
          </span>
        </div>
        <Progress value={load} />
      </div>

      <div
        className="spread"
        style={{ paddingTop: "var(--space-2)", borderTop: "1px solid var(--border-soft)" }}
      >
        <div>
          <div className="muted" style={{ fontSize: 12 }}>
            {isPaused ? "Paused — waiting on you" : agent.status === "idle" ? "Last completed" : "Working on"}
          </div>
          {taskLabel && (
            <a
              href="#"
              onClick={(e) => {
                e.preventDefault();
                if (agent.current_task_id) navigate(`/tasks/${agent.current_task_id}`);
              }}
              style={{ fontSize: 13, fontWeight: 500 }}
            >
              {taskLabel}
            </a>
          )}
        </div>
        {isPaused && <Button variant="soft" size="sm">
          Resume
        </Button>}
        {!isPaused && onEdit && (
          <Button variant="ghost" size="sm" onClick={onEdit}>
            Edit
          </Button>
        )}
      </div>
    </div>
  );
}
