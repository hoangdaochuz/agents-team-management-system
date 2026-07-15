import { useState } from "react";
import { useNavigate } from "react-router-dom";
import type { Task, TaskStatus, Priority, TaskType } from "../../api/types";
import { cn } from "../../lib/cn";
import { Icon, type IconName } from "../../lib/icons";
import { shortId } from "../../lib/format";
import { Avatar, Progress } from "../ui";

const COLUMNS: { key: TaskStatus; title: string; dot: string }[] = [
  { key: "backlog", title: "Backlog", dot: "var(--meta)" },
  { key: "doing", title: "In progress", dot: "var(--accent)" },
  { key: "review", title: "In review", dot: "var(--warn)" },
  { key: "done", title: "Done", dot: "var(--success)" },
];

const TYPE_ICON: Record<TaskType, IconName> = {
  task: "check",
  story: "sparkle",
  bug: "alert",
  epic: "board",
};

const PRIO_CLASS: Record<Priority, string> = {
  highest: "prio-highest",
  high: "prio-high",
  medium: "prio-medium",
  low: "prio-low",
};

const LABEL_CLASS: Record<string, string> = {
  backend: "lbl-backend",
  frontend: "lbl-frontend",
  bug: "lbl-bug",
  docs: "lbl-docs",
  qa: "lbl-qa",
  research: "lbl-research",
  refactor: "lbl-refactor",
};

function labelClass(lbl: string): string {
  return LABEL_CLASS[lbl.toLowerCase()] ?? "";
}

function dueState(iso: string | null | undefined): { cls: string; text: string } | null {
  if (!iso) return null;
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return null;
  const diff = t - Date.now();
  const dayMs = 86_400_000;
  if (diff < 0) return { cls: "due-over", text: "Overdue" };
  if (diff < dayMs * 2) return { cls: "due-soon", text: "Soon" };
  return null;
}

export function KanbanBoard({
  tasks,
  onMove,
}: {
  tasks: Task[];
  onMove: (taskId: string, status: TaskStatus) => void;
}) {
  const navigate = useNavigate();
  const [dragId, setDragId] = useState<string | null>(null);

  const handleDrop = (status: TaskStatus) => {
    if (dragId) onMove(dragId, status);
    setDragId(null);
  };

  return (
    <section className="kanban" data-kanban>
      {COLUMNS.map((col) => {
        const colTasks = tasks.filter((t) => t.status === col.key);
        return (
          <div
            key={col.key}
            className="col"
            data-status={col.key}
            onDragOver={(e) => {
              e.preventDefault();
              e.currentTarget.classList.add("drag-over");
            }}
            onDragLeave={(e) => e.currentTarget.classList.remove("drag-over")}
            onDrop={(e) => {
              e.currentTarget.classList.remove("drag-over");
              handleDrop(col.key);
            }}
          >
            <div className="col-head">
              <div className="col-title">
                <span className="badge-dot" style={{ background: col.dot }} />
                {col.title} <span className="col-count">{colTasks.length}</span>
              </div>
              <div className="col-head-actions">
                <button className="col-add" aria-label="Add">
                  ＋
                </button>
                <button className="col-menu" aria-label="More">
                  ⋯
                </button>
              </div>
            </div>

            <div className="col-body stack" style={{ gap: "var(--space-3)" }}>
              {colTasks.map((t) => {
                const type = (t.type ?? "task") as TaskType;
                const prio = (t.priority ?? "medium") as Priority;
                const due = dueState(t.due_at);
                const showProgress =
                  typeof t.progress === "number" && t.progress > 0 && t.status !== "done";
                return (
                  <div
                    key={t.id}
                    className={cn(
                      "tcard",
                      t.id === dragId && "dragging",
                      t.status === "done" && "done-strike",
                    )}
                    draggable
                    onDragStart={() => setDragId(t.id)}
                    onDragEnd={() => setDragId(null)}
                    onClick={() => navigate(`/tasks/${t.id}`)}
                    style={{ textDecoration: "none", color: "inherit", cursor: "pointer" }}
                  >
                    <div className="card-top">
                      <div className="card-id-row">
                        <span className={`type-icon type-${type}`}>
                          <Icon name={TYPE_ICON[type]} size={12} />
                        </span>
                        <span className="card-id">{shortId(t.id)}</span>
                      </div>
                      <span className={`prio ${PRIO_CLASS[prio]}`} title={prio}>
                        <Icon name="filter" size={13} />
                      </span>
                    </div>

                    <div className="card-title2">{t.title}</div>

                    {t.description && <div className="card-desc">{t.description}</div>}

                    {t.labels && t.labels.length > 0 && (
                      <div className="card-labels">
                        {t.labels.map((lbl) => (
                          <span key={lbl} className={`lbl ${labelClass(lbl)}`}>
                            {lbl}
                          </span>
                        ))}
                      </div>
                    )}

                    {showProgress && (
                      <div style={{ margin: "2px 0" }}>
                        <div className="spread" style={{ marginBottom: 3 }}>
                          <span className="meta-chip">
                            <Icon name="check" size={12} />
                            {t.progress}%
                          </span>
                          <span className="mono" style={{ fontSize: 11 }}>
                            {t.progress}%
                          </span>
                        </div>
                        <Progress value={t.progress ?? 0} />
                      </div>
                    )}

                    <div className="card-meta">
                      <div className="meta-left">
                        {due && (
                          <span className={`meta-chip ${due.cls}`}>
                            <Icon name="calendar" size={12} />
                            {due.text}
                          </span>
                        )}
                        {typeof t.comments_count === "number" && t.comments_count > 0 && (
                          <span className="meta-chip">
                            <Icon name="comment" size={12} />
                            {t.comments_count}
                          </span>
                        )}
                        {typeof t.attachments_count === "number" && t.attachments_count > 0 && (
                          <span className="meta-chip">
                            <Icon name="paperclip" size={12} />
                            {t.attachments_count}
                          </span>
                        )}
                      </div>
                      <div className="meta-left">
                        {typeof t.points === "number" && <span className="points">{t.points}</span>}
                        {t.agent_id && <Avatar name={t.agent_id} size="sm" />}
                      </div>
                    </div>
                  </div>
                );
              })}

              <button className="col-add-card">+ Create issue</button>
            </div>
          </div>
        );
      })}
    </section>
  );
}
