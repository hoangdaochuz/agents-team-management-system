import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  AsyncBoundary,
  Avatar,
  Badge,
  Button,
  Card,
  Field,
  Input,
  Select,
  StatusBadge,
} from "../components/ui";
import { Icon } from "../lib/icons";
import { betweenMs, formatDuration, relativeTime, shortId } from "../lib/format";
import { agents, tasks } from "../api/client";
import type { Agent, Task } from "../api/types";
import { useActiveWorkspaceId } from "../lib/workspace";

// ── Filters ──────────────────────────────────────────────────────────
type StatusFilter = "all" | "success" | "revised" | "failed";

const STATUS_OPTIONS: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "success", label: "Success" },
  { value: "revised", label: "Revised" },
  { value: "failed", label: "Failed" },
];

// Map a Task's lifecycle status into the history "outcome" buckets the
// filter exposes. The backend doesn't yet expose run-level status on the
// task row, so this is an approximation derived from Task.status.
function taskOutcome(t: Task): StatusFilter {
  if (t.status === "done") return "success";
  if (t.status === "review") return "revised";
  if (t.status === "blocked" || t.status === "stopped" || t.status === "cancelled")
    return "failed";
  return "success"; // unknown → treat as success for filter purposes
}

const PAGE_SIZE = 10;

// ── CSV export ───────────────────────────────────────────────────────
function downloadCSV(filename: string, rows: Record<string, string | number | null>[]) {
  if (!rows.length) return;
  const headers = Object.keys(rows[0]);
  const escape = (v: string | number | null) => {
    const s = v == null ? "" : String(v);
    return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
  };
  const csv = [
    headers.join(","),
    ...rows.map((r) => headers.map((h) => escape(r[h] ?? null)).join(",")),
  ].join("\n");
  const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

interface HistoryRow {
  task: Task;
  agent?: Agent;
  outcome: StatusFilter;
  durationMs: number | null;
  feedback: string;
  outcomeText: string;
}

export function HistoryPage() {
  const navigate = useNavigate();
  // Scope cached data to the active workspace so switching refetches (design D3).
  const wid = useActiveWorkspaceId();

  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [agentFilter, setAgentFilter] = useState<string>("all");
  const [from, setFrom] = useState<string>("");
  const [to, setTo] = useState<string>("");
  const [page, setPage] = useState(0);

  // Apply/reset re-derives the filtered list; these just force a re-render
  // by bumping an internal counter via the dependency in useMemo below.
  const [applied, setApplied] = useState(0);

  const tasksQuery = useQuery({
    queryKey: ["tasks", wid, "history"],
    queryFn: () => tasks.listTasks(),
  });

  const agentsQuery = useQuery({
    queryKey: ["agents", wid, "history"],
    queryFn: () => agents.listAgents(),
  });

  const agentById = useMemo(() => {
    const m = new Map<string, Agent>();
    (agentsQuery.data ?? []).forEach((a) => m.set(a.id, a));
    return m;
  }, [agentsQuery.data]);

  // Build display rows from tasks. There's no global runs endpoint, so each
  // Task becomes a row; status/duration/outcome are derived or "—".
  const rows: HistoryRow[] = useMemo(() => {
    return (tasksQuery.data ?? []).map((t) => {
      const agent = t.agent_id ? agentById.get(t.agent_id) : undefined;
      const durationMs =
        t.updated_at && t.created_at ? betweenMs(t.created_at, t.updated_at) : null;
      return {
        task: t,
        agent,
        outcome: taskOutcome(t),
        durationMs,
        feedback: t.comments_count ? `${t.comments_count} note${t.comments_count > 1 ? "s" : ""}` : "—",
        outcomeText:
          t.status === "done"
            ? `done · ${t.progress ?? 100}%`
            : t.status === "review"
              ? "up for review"
              : t.status,
      };
    });
  }, [tasksQuery.data, agentById]);

  // Client-side filtering.
  const filtered = useMemo(() => {
    return rows.filter((r) => {
      if (statusFilter !== "all" && r.outcome !== statusFilter) return false;
      if (agentFilter !== "all" && r.task.agent_id !== agentFilter) return false;
      const created = r.task.created_at ? new Date(r.task.created_at).getTime() : NaN;
      if (from && Number.isFinite(created) && created < new Date(from).getTime()) return false;
      // 'to' is inclusive of the day: add one day so the boundary covers it.
      if (to && Number.isFinite(created)) {
        const end = new Date(to).getTime() + 24 * 60 * 60 * 1000;
        if (created > end) return false;
      }
      return true;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rows, statusFilter, agentFilter, from, to, applied]);

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount - 1);
  const paged = filtered.slice(safePage * PAGE_SIZE, safePage * PAGE_SIZE + PAGE_SIZE);

  function applyFilters() {
    setPage(0);
    setApplied((n) => n + 1);
  }
  function resetFilters() {
    setStatusFilter("all");
    setAgentFilter("all");
    setFrom("");
    setTo("");
    setPage(0);
    setApplied((n) => n + 1);
  }

  function handleExport() {
    downloadCSV(
      "run-history.csv",
      filtered.map((r) => ({
        Task: shortId(r.task.id),
        Title: r.task.title,
        Agent: r.agent?.name ?? "—",
        Status: r.outcome,
        Duration: formatDuration(r.durationMs),
        Feedback: r.feedback,
        Outcome: r.outcomeText,
        When: relativeTime(r.task.updated_at ?? r.task.created_at),
      })),
    );
  }

  const agentOptions = agentsQuery.data ?? [];

  return (
    <>
      <div className="page-head">
        <div>
          <h1 className="page-title">Run history</h1>
          <div className="page-sub">
            Every time an agent ran a task — including re-runs from feedback
          </div>
        </div>
        <Button variant="ghost" icon={<Icon name="download" size={16} />} onClick={handleExport}>
          Export CSV
        </Button>
      </div>

      {/* Filters */}
      <Card className="mt-6" >
        <div className="grid" style={{ gridTemplateColumns: "repeat(4, 1fr)" }}>
          <Field label="Status">
            <Select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value as StatusFilter)}
            >
              {STATUS_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Agent">
            <Select value={agentFilter} onChange={(e) => setAgentFilter(e.target.value)}>
              <option value="all">All agents</option>
              {agentOptions.length === 0 && (
                <>
                  <option value="atlas">Atlas</option>
                  <option value="forge">Forge</option>
                  <option value="echo">Echo</option>
                  <option value="iris">Iris</option>
                  <option value="nova">Nova</option>
                </>
              )}
              {agentOptions.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="From">
            <Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
          </Field>
          <Field label="To">
            <Input type="date" value={to} onChange={(e) => setTo(e.target.value)} />
          </Field>
        </div>
        <div className="row mt-4" style={{ gap: "var(--space-2)" }}>
          <Button variant="primary" size="sm" onClick={applyFilters}>
            Apply
          </Button>
          <Button variant="ghost" size="sm" onClick={resetFilters}>
            Reset
          </Button>
        </div>
      </Card>

      {/* Table */}
      <Card flush>
        <div className="table-scroll">
          <table className="table">
            <thead>
              <tr>
                <th>Task</th>
                <th>Agent</th>
                <th>Status</th>
                <th>Duration</th>
                <th>Feedback</th>
                <th>Outcome</th>
                <th>When</th>
              </tr>
            </thead>
            <tbody>
              <AsyncBoundary
                isLoading={tasksQuery.isLoading}
                isError={tasksQuery.isError}
                error={tasksQuery.error}
                data={paged}
                isEmpty={(d) => d.length === 0}
                emptyTitle="No runs match"
                emptyHint="Adjust the filters above, or wait for the first agent runs to come in."
              >
                {(data) =>
                  data.map((r) => (
                    <tr
                      key={r.task.id}
                      className="link-row"
                      onClick={() => navigate(`/tasks/${r.task.id}`)}
                    >
                      <td>
                        <span style={{ fontWeight: 500 }}>{shortId(r.task.id)}</span>
                        <div className="muted" style={{ fontSize: 12 }}>
                          {r.task.title}
                        </div>
                      </td>
                      <td>
                        {r.agent ? (
                          <div className="row" style={{ alignItems: "center", gap: 8 }}>
                            <Avatar name={r.agent.name} size="sm" />
                            {r.agent.name}
                          </div>
                        ) : (
                          <span className="muted">—</span>
                        )}
                      </td>
                      <td>
                        <StatusBadge status={r.task.status} />
                      </td>
                      <td className="t-mono">{formatDuration(r.durationMs)}</td>
                      <td>
                        {r.feedback === "—" ? (
                          <Badge tone="muted">—</Badge>
                        ) : (
                          <Badge tone={r.outcome === "revised" ? "accent" : "muted"}>
                            {r.feedback}
                          </Badge>
                        )}
                      </td>
                      <td>{r.outcomeText}</td>
                      <td className="muted">{relativeTime(r.task.updated_at ?? r.task.created_at)}</td>
                    </tr>
                  ))
                }
              </AsyncBoundary>
            </tbody>
          </table>
        </div>
      </Card>

      {/* Pagination */}
      <div className="spread mt-6">
        <span className="muted" style={{ fontSize: 13 }}>
          Showing {filtered.length === 0 ? 0 : safePage * PAGE_SIZE + 1}–
          {Math.min((safePage + 1) * PAGE_SIZE, filtered.length)} / {filtered.length} runs in the
          selected range
        </span>
        <div className="row">
          <Button
            variant="ghost"
            size="sm"
            disabled={safePage === 0}
            onClick={() => setPage((p) => Math.max(0, p - 1))}
          >
            Previous
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={safePage >= pageCount - 1}
            onClick={() => setPage((p) => p + 1)}
          >
            Next
          </Button>
        </div>
      </div>
    </>
  );
}
