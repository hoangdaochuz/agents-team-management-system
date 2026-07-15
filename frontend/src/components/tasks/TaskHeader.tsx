import type { Task } from "../../api/types";
import { shortId, relativeTime, betweenMs, formatDuration } from "../../lib/format";
import { Avatar, Badge, Button, Progress, StatusBadge } from "../ui";
import { Icon } from "../../lib/icons";

const PRIO_TONE = {
  highest: "danger",
  high: "warn",
  medium: "muted",
  low: "muted",
} as const;

const ASSIGNEE_NAME = "Forge"; // resolved agent name; backend lookup not implemented yet

export function TaskHeader({
  task,
  paused,
  onPause,
  onStop,
  onReview,
}: {
  task?: Task;
  paused: boolean;
  onPause: () => void;
  onStop: () => void;
  onReview: () => void;
}) {
  const title = task?.title ?? "Untitled task";
  const description = task?.description ?? task?.prompt ?? "";
  const status = paused ? "paused" : task?.status ?? "backlog";
  const progress = task?.progress ?? 0;
  const prio = task?.priority ?? "medium";

  const startedAgo = task ? relativeTime(task.created_at) : "—";
  const etaMs = task ? betweenMs(task.created_at, task?.due_at ?? null) : null;
  const eta = formatDuration(etaMs);

  return (
    <section className="card" style={{ marginBottom: "var(--space-5)" }}>
      <div className="spread" style={{ alignItems: "flex-start", flexWrap: "wrap", gap: "var(--space-4)" }}>
        <div style={{ minWidth: 260 }}>
          <div className="row" style={{ marginBottom: 8 }}>
            <StatusBadge status={status} />
            <Badge tone="muted">{task ? shortId(task.id) : "TS-···"}</Badge>
            <Badge tone={PRIO_TONE[prio]} dot>
              {prio.toUpperCase()}
            </Badge>
          </div>
          <h1 className="page-title">{title}</h1>
          {description && (
            <p className="muted mt-2" style={{ fontSize: 14, maxWidth: 620 }}>
              {description}
            </p>
          )}
        </div>

        <div className="row" style={{ alignItems: "flex-start" }}>
          {!paused && (
            <Button variant="ghost" icon={<Icon name="pause" size={16} />} onClick={onPause}>
              Pause &amp; give feedback
            </Button>
          )}
          <Button variant="soft" icon={<Icon name="stop" size={15} />} onClick={onStop}>
            Stop
          </Button>
          <Button variant="primary" icon={<Icon name="check" size={16} />} onClick={onReview}>
            Review output
          </Button>
        </div>
      </div>

      <hr style={{ border: "none", borderTop: "1px solid var(--border-soft)", margin: "var(--space-5) 0" }} />

      <div className="spread" style={{ flexWrap: "wrap", gap: "var(--space-5)" }}>
        <div className="row" style={{ alignItems: "center", gap: 12 }}>
          <Avatar name={ASSIGNEE_NAME} size="lg" />
          <div>
            <div style={{ fontWeight: 600 }}>{ASSIGNEE_NAME}</div>
            <div className="muted" style={{ fontSize: 13 }}>
              Code refactorer · owns this task
            </div>
          </div>
        </div>

        <div className="stack" style={{ gap: 2 }}>
          <div className="muted" style={{ fontSize: 12, textTransform: "uppercase", letterSpacing: ".05em" }}>
            Progress
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <Progress value={progress} className="progress-mini" />
            <span className="mono">{progress}%</span>
          </div>
        </div>

        <div className="stack" style={{ gap: 2 }}>
          <div className="muted" style={{ fontSize: 12, textTransform: "uppercase", letterSpacing: ".05em" }}>
            Started
          </div>
          <div className="mono">{startedAgo}</div>
        </div>

        <div className="stack" style={{ gap: 2 }}>
          <div className="muted" style={{ fontSize: 12, textTransform: "uppercase", letterSpacing: ".05em" }}>
            ETA
          </div>
          <div className="mono">~{eta}</div>
        </div>
      </div>
    </section>
  );
}
