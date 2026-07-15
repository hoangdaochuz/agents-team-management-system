import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { tasks, agents } from "../api/client";
import type { Task, TaskStatus } from "../api/types";
import {
  AsyncBoundary,
  AvatarStack,
  Button,
  Segmented,
  useToast,
} from "../components/ui";
import { Icon } from "../lib/icons";
import { KanbanBoard } from "../components/tasks/KanbanBoard";
import { CreateTaskModal } from "../components/tasks/CreateTaskModal";

export function KanbanPage() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [search, setSearch] = useState("");
  const [view, setView] = useState<"board" | "list" | "timeline">("board");
  const [creating, setCreating] = useState(false);

  const { data, isLoading, isError, error } = useQuery<Task[]>({
    queryKey: ["tasks"],
    queryFn: () => tasks.listTasks(),
  });

  // Assignee names for the toolbar avatar stack. Best-effort — tolerates the
  // agents endpoint being absent (the kanban still renders).
  const { data: agentList } = useQuery({
    queryKey: ["agents"],
    queryFn: () => agents.listAgents(),
  });

  const move = useMutation({
    mutationFn: ({ id, status }: { id: string; status: TaskStatus }) =>
      tasks.patchStatus(id, status),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["tasks"] });
    },
    onError: (e: unknown) => {
      const msg = e instanceof Error ? e.message : "Move failed";
      toast(msg, "alert");
    },
  });

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return data ?? [];
    return (data ?? []).filter(
      (t) =>
        t.title.toLowerCase().includes(q) ||
        (t.description ?? "").toLowerCase().includes(q) ||
        (t.labels ?? []).some((l) => l.toLowerCase().includes(q)),
    );
  }, [data, search]);

  const assigneeNames = useMemo(
    () => (agentList ?? []).map((a) => a.name),
    [agentList],
  );

  return (
    <>
      <div className="page-head">
        <div>
          <div className="crumbs" style={{ marginBottom: 6 }}>
            <a href="#/dashboard">Projects</a>
            <span className="sep">/</span>
            <span>Agent Ops</span>
            <span className="sep">/</span>
            <span className="fg2">Sprint 24 board</span>
          </div>
          <h1 className="page-title">Board</h1>
          <div className="page-sub">
            {filtered.length} issues · drag cards across columns to update status · each issue is owned by one agent
          </div>
        </div>
        <Button variant="primary" onClick={() => setCreating(true)}>
          + New task
        </Button>
      </div>

      {/* BOARD TOOLBAR */}
      <div className="board-toolbar">
        <div className="board-search">
          <Icon name="search" size={15} style={{ color: "var(--meta)" }} />
          <input
            type="text"
            placeholder="Search issues…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>

        <button className="filter-btn">
          <Icon name="filter" size={15} />
          Type
        </button>
        <button className="filter-btn">Priority</button>
        <button className="filter-btn">Assignee</button>
        <button className="filter-btn">Labels</button>

        <span className="divider-v" />

        <button className="filter-btn">
          Group: <b style={{ color: "var(--fg)", marginLeft: 2 }}>Status</b>
          <Icon name="chevronDown" size={13} />
        </button>

        <span className="divider-v" />

        <AvatarStack names={assigneeNames} />

        <div style={{ marginLeft: "auto" }}>
          <Segmented
            value={view}
            onChange={setView}
            options={[
              { value: "board", label: "Board" },
              { value: "list", label: "List" },
              { value: "timeline", label: "Timeline" },
            ]}
          />
        </div>
      </div>

      <AsyncBoundary
        isLoading={isLoading}
        isError={isError}
        error={error}
        data={data}
        isEmpty={(d: Task[]) => d.length === 0}
        emptyTitle="No issues on this board"
        emptyHint="Create your first issue — it lands in the Backlog column."
      >
        {() => (
          <KanbanBoard
            tasks={filtered}
            onMove={(id, status) => move.mutate({ id, status })}
          />
        )}
      </AsyncBoundary>

      <p className="muted mt-6" style={{ fontSize: 13 }}>
        Tip: drag a card to another column to change its status. Dropping into <b>In review</b> pings you — the agent won't continue until you respond.
      </p>

      <CreateTaskModal open={creating} onClose={() => setCreating(false)} />
    </>
  );
}
