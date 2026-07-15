import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { tasks, agents } from "../api/client";
import type { Task, Agent } from "../api/types";
import { Avatar, Badge, Card, CardHead, KPI, Progress, Sparkline, StatusBadge } from "../components/ui";
import { relativeTime, shortId } from "../lib/format";

/** Map an agent status string to a display tone. */
function agentTone(status?: string): "success" | "warn" | "muted" {
  if (status === "running") return "success";
  if (status === "paused") return "warn";
  return "muted";
}

export function DashboardPage() {
  const navigate = useNavigate();

  const tasksQuery = useQuery({
    queryKey: ["tasks", { status: "doing" }],
    queryFn: () => tasks.listTasks({ status: "doing" }),
  });

  const reviewQuery = useQuery({
    queryKey: ["tasks", { status: "review" }],
    queryFn: () => tasks.listTasks({ status: "review" }),
  });

  const agentsQuery = useQuery({
    queryKey: ["agents"],
    queryFn: () => agents.listAgents(),
  });

  // Activity feed is derived from recent tasks (newest first) since the backend
  // has no dedicated activity endpoint yet. Wrapped in AsyncBoundary.
  const activityQuery = useQuery({
    queryKey: ["tasks", "recent"],
    queryFn: () => tasks.listTasks(),
  });

  const running = tasksQuery.data ?? [];
  const reviewQueue = reviewQuery.data ?? [];
  const allAgents = agentsQuery.data ?? [];

  const runningCount = running.length;
  const activeAgents = allAgents.filter((a) => a.status === "running").length;
  const doneToday = (activityQuery.data ?? []).filter(
    (t) => t.status === "done",
  ).length;
  const awaitingFeedback = reviewQueue.length;

  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">Dashboard</h1>
          <div className="page-sub">Workspace overview · updated in real time</div>
        </div>
        <div className="row">
          <button className="btn btn-ghost" onClick={() => navigate("/history")}>
            View history
          </button>
          <button className="btn btn-primary" onClick={() => navigate("/board")}>
            Open board
          </button>
        </div>
      </div>

      {/* KPI row */}
      <section className="kpi-row">
        <KPI
          label="Active agents"
          value={
            <>
              {activeAgents}
              <span style={{ fontSize: 18, color: "var(--muted)" }}> / {Math.max(allAgents.length, 5)}</span>
            </>
          }
          delta={activeAgents > 0 ? `${activeAgents} online` : "—"}
          trend={activeAgents > 0 ? "up" : undefined}
          spark={<Sparkline points={[3, 2, 4, 3, 5, 4, activeAgents]} />}
        />
        <KPI
          label="Running tasks"
          value={runningCount}
          delta={awaitingFeedback > 0 ? `${awaitingFeedback} up for review` : "none in review"}
          spark={<Sparkline points={[2, 3, 1, 4, 5, 3, Math.max(runningCount, 1)]} />}
        />
        <KPI
          label="Completed today"
          value={doneToday}
          delta={doneToday > 0 ? "▲ +tasks finalized" : "no completions yet"}
          trend={doneToday > 0 ? "up" : undefined}
          spark={<Sparkline points={[0, 1, 2, 1, 3, 4, Math.max(doneToday, 1)]} />}
        />
        <KPI
          label="Awaiting feedback"
          value={awaitingFeedback}
          delta={awaitingFeedback > 0 ? "tasks waiting on you" : "all clear"}
          trend={awaitingFeedback > 0 ? "down" : undefined}
          spark={<Sparkline points={[1, 2, 1, 3, 2, 4, Math.max(awaitingFeedback, 1)]} />}
        />
      </section>

      {/* Main grid */}
      <div className="grid mt-6" style={{ gridTemplateColumns: "1.6fr 1fr" }}>
        {/* LEFT column */}
        <div className="stack">
          <Card>
            <CardHead
              title="Running tasks"
              link={
                <span
                  className="card-link"
                  role="link"
                  tabIndex={0}
                  style={{ cursor: "pointer" }}
                  onClick={() => navigate("/board")}
                >
                  View all →
                </span>
              }
            />
            <RunningTasksSection query={tasksQuery} onOpen={(id) => navigate(`/tasks/${id}`)} />
          </Card>

          <Card>
            <CardHead title="Recent activity" />
            <ActivitySection query={activityQuery} onOpenTask={(id) => navigate(`/tasks/${id}`)} />
          </Card>
        </div>

        {/* RIGHT column */}
        <div className="stack">
          <Card>
            <CardHead
              title="Agent status"
              link={
                <span
                  className="card-link"
                  role="link"
                  tabIndex={0}
                  style={{ cursor: "pointer" }}
                  onClick={() => navigate("/agents")}
                >
                  Manage →
                </span>
              }
            />
            <AgentStatusSection query={agentsQuery} />
          </Card>

          <Card>
            <CardHead title="Waiting on your review" />
            <ReviewQueueSection query={reviewQuery} onOpen={(id) => navigate(`/tasks/${id}`)} />
          </Card>
        </div>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ *
 * Section sub-components — each wraps its React Query result in an
 * AsyncBoundary so the layout chrome renders even when the backend
 * returns 404 (every endpoint except /healthz is unimplemented today).
 * ------------------------------------------------------------------ */

import { AsyncBoundary } from "../components/ui";

/** Running tasks: card list with progress + status badge. */
function RunningTasksSection({
  query,
  onOpen,
}: {
  query: ReturnType<typeof useQuery<Task[]>>;
  onOpen: (id: string) => void;
}) {
  return (
    <AsyncBoundary<Task[]>
      isLoading={query.isLoading}
      isError={query.isError}
      error={query.error}
      data={query.data}
      isEmpty={(d) => !d || d.length === 0}
      emptyTitle="No running tasks"
      emptyHint="Tasks move here once an agent starts working on them."
    >
      {(list) => (
        <div className="stack" style={{ gap: "var(--space-3)" }}>
          {list.map((t) => (
            <div
              key={t.id}
              className="tcard"
              role="link"
              tabIndex={0}
              style={{ cursor: "pointer" }}
              onClick={() => onOpen(t.id)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onOpen(t.id);
                }
              }}
            >
              <div className="spread">
                <div>
                  <div className="tcard-title">{t.title}</div>
                  <div className="muted" style={{ fontSize: 12, marginTop: 2 }}>
                    {shortId(t.id)} · updated {relativeTime(t.updated_at)}
                  </div>
                </div>
                <StatusBadge status={t.status} pulse={t.status === "doing"} />
              </div>
              {typeof t.progress === "number" && (
                <div>
                  <div className="spread" style={{ marginBottom: 5 }}>
                    <span className="fg2" style={{ fontSize: 12 }}>
                      {t.branch_name ? `branch ${t.branch_name}` : "in progress"}
                    </span>
                    <span className="mono" style={{ fontSize: 12 }}>
                      {Math.round(t.progress)}%
                    </span>
                  </div>
                  <Progress value={t.progress} />
                </div>
              )}
              <div className="tcard-foot">
                <div className="row" style={{ alignItems: "center", gap: 8 }}>
                  <Avatar name={agentNameFor(t)} text={agentNameFor(t)} size="sm" />
                  <span className="fg2" style={{ fontSize: 13 }}>
                    {agentNameFor(t)}
                  </span>
                </div>
                <span className="due">
                  {t.due_at ? `due ${relativeTime(t.due_at)}` : "—"}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </AsyncBoundary>
  );
}

/** Activity feed derived from recent tasks. */
function ActivitySection({
  query,
  onOpenTask,
}: {
  query: ReturnType<typeof useQuery<Task[]>>;
  onOpenTask: (id: string) => void;
}) {
  return (
    <AsyncBoundary<Task[]>
      isLoading={query.isLoading}
      isError={query.isError}
      error={query.error}
      data={query.data}
      isEmpty={(d) => !d || d.length === 0}
      emptyTitle="No activity yet"
      emptyHint="Runs and task updates will stream in here."
    >
      {(list) => (
        <div className="feed">
          {list.slice(0, 6).map((t) => {
            const name = agentNameFor(t);
            return (
              <div className="feed-item" key={t.id}>
                <div className="feed-dot">
                  <Avatar name={name} text={name} size="sm" />
                </div>
                <div>
                  <div className="feed-text">
                    <b>{name}</b> updated{" "}
                    <a
                      href={`#/tasks/${t.id}`}
                      onClick={(e) => {
                        e.preventDefault();
                        onOpenTask(t.id);
                      }}
                    >
                      {shortId(t.id)}
                    </a>{" "}
                    · {t.title}
                  </div>
                  <div className="feed-time">{relativeTime(t.updated_at)}</div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </AsyncBoundary>
  );
}

/** Agent status: roster with status badges. */
function AgentStatusSection({
  query,
}: {
  query: ReturnType<typeof useQuery<Agent[]>>;
}) {
  return (
    <AsyncBoundary<Agent[]>
      isLoading={query.isLoading}
      isError={query.isError}
      error={query.error}
      data={query.data}
      isEmpty={(d) => !d || d.length === 0}
      emptyTitle="No agents registered"
      emptyHint="Create agents in Settings to see them here."
    >
      {(list) => (
        <div className="stack" style={{ gap: "var(--space-3)" }}>
          {list.map((a) => (
            <div className="spread" key={a.id}>
              <div className="row" style={{ alignItems: "center", gap: 10 }}>
                <Avatar name={a.name} />
                <div>
                  <div style={{ fontWeight: 600, fontSize: 14 }}>{a.name}</div>
                  <div className="muted" style={{ fontSize: 12 }}>
                    {a.role}
                  </div>
                </div>
              </div>
              <Badge tone={agentTone(a.status)} dot>
                {a.status ?? "idle"}
              </Badge>
            </div>
          ))}
        </div>
      )}
    </AsyncBoundary>
  );
}

/** Review queue: tasks awaiting human review. */
function ReviewQueueSection({
  query,
  onOpen,
}: {
  query: ReturnType<typeof useQuery<Task[]>>;
  onOpen: (id: string) => void;
}) {
  return (
    <AsyncBoundary<Task[]>
      isLoading={query.isLoading}
      isError={query.isError}
      error={query.error}
      data={query.data}
      isEmpty={(d) => !d || d.length === 0}
      emptyTitle="Review queue is empty"
      emptyHint="Tasks awaiting your feedback appear here."
    >
      {(list) => (
        <div className="stack" style={{ gap: "var(--space-3)" }}>
          {list.map((t) => (
            <div
              key={t.id}
              className="spread"
              role="link"
              tabIndex={0}
              style={{
                cursor: "pointer",
                padding: "var(--space-3)",
                borderRadius: "var(--radius-md)",
                background: "var(--surface)",
              }}
              onClick={() => onOpen(t.id)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onOpen(t.id);
                }
              }}
            >
              <div>
                <div style={{ fontWeight: 600, fontSize: 14 }}>
                  {shortId(t.id)} · {t.title}
                </div>
                <div className="muted" style={{ fontSize: 12 }}>
                  {agentNameFor(t)}
                  {typeof t.progress === "number" ? ` · ${Math.round(t.progress)}%` : ""}
                </div>
              </div>
              <Badge tone="accent">review</Badge>
            </div>
          ))}
        </div>
      )}
    </AsyncBoundary>
  );
}

/* ---- helpers ---- */

const PLACEHOLDER_AGENT = "Agent";

/** Derive a display name for a task's assignee, or a placeholder. */
function agentNameFor(t: Task): string {
  if (t.agent_id) return PLACEHOLDER_AGENT;
  return PLACEHOLDER_AGENT;
}
