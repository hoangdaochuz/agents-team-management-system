import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import {
  knowledgeSources,
  plugins,
  rules,
  skills,
  workspaceMcp,
} from "../api/client";
import type { KnowledgeSource, McpConnection, Plugin, Rule, Skill } from "../api/types";
import { useActiveWorkspaceId } from "../lib/workspace";
import { useToast } from "../components/ui";
import { Switch } from "../components/ui/Switch";
import { Tabs, type TabDef } from "../components/ui/Tabs";
import { WorkspaceScopeBanner } from "../components/workspaces/WorkspaceScopeBanner";
import { ResourceList } from "../components/resources/ResourceList";
import { Button } from "../components/ui/Button";

export function WorkspaceResourcesPage() {
  // Honor an explicit :id deep link, falling back to the session's active workspace.
  const { id } = useParams();
  const activeId = useActiveWorkspaceId();
  const wid = id ?? activeId;
  const { toast } = useToast();

  const tabs: TabDef[] = [
    { key: "kb", label: "Knowledge base", content: <KnowledgeTab wid={wid} toast={toast} /> },
    { key: "skills", label: "Skills", content: <SkillsTab wid={wid} toast={toast} /> },
    { key: "plugins", label: "Plugins", content: <PluginsTab wid={wid} toast={toast} /> },
    { key: "mcp", label: "MCP servers", content: <McpTab wid={wid} toast={toast} /> },
    { key: "rules", label: "Rules", content: <RulesTab wid={wid} toast={toast} /> },
  ];

  return (
    <>
      <div className="crumbs">
        <Link to="/workspaces">Workspaces</Link>
        <span className="sep">/</span>
        <span>Resources</span>
      </div>

      <div className="page-head">
        <div>
          <h1 className="page-title">Workspace resources</h1>
          <div className="page-sub">Knowledge, skills, plugins, MCP servers, and rules for this workspace</div>
        </div>
        <Link to="/agents/builder" className="btn btn-ghost">
          + New agent
        </Link>
      </div>

      <WorkspaceScopeBanner />

      <section className="card">
        <Tabs tabs={tabs} />
      </section>
    </>
  );
}

type ToastFn = (msg: string, icon?: "check" | "alert" | "bell") => void;

/* ── Knowledge base ─────────────────────────────────────────────── */
function KnowledgeTab({ wid, toast }: { wid: string | undefined; toast: ToastFn }) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["kb", wid],
    queryFn: () => knowledgeSources.list(wid!),
    enabled: Boolean(wid),
  });
  if (isLoadingOrErr(isLoading, isError)) return loadingOrErr();
  return (
    <ResourceList<KnowledgeSource>
      items={data ?? []}
      icon="book"
      title={(it) => it.title}
      meta={(it) => `${it.kind} · ${it.chunks ?? it.pages ?? 0} ${it.chunks ? "chunks" : "pages"}`}
      status={(it) =>
        it.status === "indexed"
          ? { tone: "success", label: "Indexed" }
          : it.status === "reindexing"
            ? { tone: "warn", label: "Re-indexing", dot: true }
            : it.status === "failed"
              ? { tone: "danger", label: "Failed" }
              : { tone: "muted", label: "Pending" }
      }
      addLabel="+ Add source"
      onAdd={() => toast("Importing sources isn't implemented yet.", "alert")}
      searchPlaceholder="Search sources…"
    />
  );
}

/* ── Skills ─────────────────────────────────────────────────────── */
function SkillsTab({ wid, toast }: { wid: string | undefined; toast: ToastFn }) {
  const qc = useQueryClient();
  const { data, isLoading, isError } = useQuery({
    queryKey: ["ws-skills", wid],
    queryFn: () => skills.listForWorkspace(wid!),
    enabled: Boolean(wid),
  });
  const toggle = useToggle<Skill>({
    qc,
    key: ["ws-skills", wid],
    mutate: (id, enabled) => skills.setEnabled(wid!, id, enabled),
    toast,
  });
  if (isLoadingOrErr(isLoading, isError)) return loadingOrErr();
  return (
    <ResourceList<Skill>
      items={data ?? []}
      icon="sparkle"
      title={(it) => it.name}
      meta={(it) =>
        [it.trigger && `Trigger: ${it.trigger}`, it.step_count && `${it.step_count} steps`, it.description]
          .filter(Boolean)
          .join(" · ")
      }
      status={(it) =>
        it.enabled ? { tone: "accent", label: "Enabled" } : { tone: "muted", label: "Disabled" }
      }
      trailing={(it) => (
        <Switch checked={Boolean(it.enabled)} onChange={(v) => toggle.mutate({ id: it.id, enabled: v })} label={`Toggle ${it.name}`} />
      )}
      addLabel="+ Add skill"
      onAdd={() => toast("Adding skills isn't implemented yet.", "alert")}
      searchPlaceholder="Search skills…"
    />
  );
}

/* ── Plugins ────────────────────────────────────────────────────── */
function PluginsTab({ wid, toast }: { wid: string | undefined; toast: ToastFn }) {
  const qc = useQueryClient();
  const { data, isLoading, isError } = useQuery({
    queryKey: ["plugins", wid],
    queryFn: () => plugins.list(wid!),
    enabled: Boolean(wid),
  });
  const toggle = useToggle<Plugin>({
    qc,
    key: ["plugins", wid],
    mutate: (id, enabled) => plugins.setEnabled(wid!, id, enabled),
    toast,
  });
  if (isLoadingOrErr(isLoading, isError)) return loadingOrErr();
  return (
    <ResourceList<Plugin>
      items={data ?? []}
      icon="puzzle"
      title={(it) => it.name}
      meta={(it) => `v${it.version} · ${(it.capabilities ?? []).join(", ") || "no capabilities"}`}
      status={(it) =>
        it.enabled ? { tone: "accent", label: "Enabled" } : { tone: "muted", label: "Disabled" }
      }
      trailing={(it) => (
        <Switch checked={it.enabled} onChange={(v) => toggle.mutate({ id: it.id, enabled: v })} label={`Toggle ${it.name}`} />
      )}
      addLabel="+ Install plugin"
      onAdd={() => toast("Installing plugins isn't implemented yet.", "alert")}
      searchPlaceholder="Search plugins…"
    />
  );
}

/* ── MCP servers ────────────────────────────────────────────────── */
function McpTab({ wid, toast }: { wid: string | undefined; toast: ToastFn }) {
  const qc = useQueryClient();
  const { data, isLoading, isError } = useQuery({
    queryKey: ["ws-mcp", wid],
    queryFn: () => workspaceMcp.list(wid!),
    enabled: Boolean(wid),
  });
  const reconnect = useMutation({
    mutationFn: (id: string) => workspaceMcp.reconnect(wid!, id),
    onMutate: (id) => optimistic(qc, ["ws-mcp", wid], (list: McpConnection[]) =>
      list.map((m) => (m.id === id ? { ...m, status: "connected" as const } : m))),
    onSuccess: () => toast("Reconnect requested.", "check"),
    onError: () => toast("Couldn't reach the backend.", "alert"),
  });
  if (isLoadingOrErr(isLoading, isError)) return loadingOrErr();
  return (
    <ResourceList<McpConnection>
      items={data ?? []}
      icon="server"
      title={(it) => it.name}
      meta={(it) =>
        [
          `${it.transport} · ${it.tool_count} tools`,
          it.tool_names && it.tool_names.length > 0 ? it.tool_names.join(", ") : null,
        ]
          .filter(Boolean)
          .join(" · ")
      }
      status={(it) =>
        it.status === "connected"
          ? { tone: "success", label: "Connected" }
          : { tone: "danger", label: "Offline" }
      }
      trailing={(it) =>
        it.status === "offline" ? (
          <Button variant="ghost" size="sm" onClick={() => reconnect.mutate(it.id)}>
            Reconnect
          </Button>
        ) : undefined
      }
      addLabel="+ Connect server"
      onAdd={() => toast("Connecting servers isn't implemented yet.", "alert")}
      searchPlaceholder="Search servers…"
    />
  );
}

/* ── Rules ──────────────────────────────────────────────────────── */
function RulesTab({ wid, toast }: { wid: string | undefined; toast: ToastFn }) {
  const qc = useQueryClient();
  const { data, isLoading, isError } = useQuery({
    queryKey: ["rules", wid],
    queryFn: () => rules.list(wid!),
    enabled: Boolean(wid),
  });
  const toggle = useToggle<Rule>({
    qc,
    key: ["rules", wid],
    mutate: (id, enabled) => rules.setEnabled(wid!, id, enabled),
    toast,
  });
  if (isLoadingOrErr(isLoading, isError)) return loadingOrErr();
  return (
    <ResourceList<Rule>
      items={data ?? []}
      icon="shield"
      title={(it) => it.name}
      meta={(it) => it.description ?? "Workspace rule"}
      status={(it) =>
        it.enabled ? { tone: "accent", label: "Enforced" } : { tone: "muted", label: "Off" }
      }
      trailing={(it) => (
        <Switch checked={it.enabled} onChange={(v) => toggle.mutate({ id: it.id, enabled: v })} label={`Toggle ${it.name}`} />
      )}
      addLabel="+ Add rule"
      onAdd={() => toast("Adding rules isn't implemented yet.", "alert")}
      searchPlaceholder="Search rules…"
    />
  );
}

/* ── helpers ────────────────────────────────────────────────────── */
function isLoadingOrErr(isLoading: boolean, isError: boolean) {
  return isLoading || isError;
}
function loadingOrErr() {
  return (
    <div className="muted" style={{ padding: "var(--space-6)" }}>
      Loading… (endpoints aren't implemented yet — the UI still renders against the declared contract.)
    </div>
  );
}

function optimistic<T>(qc: ReturnType<typeof useQueryClient>, key: unknown[], fn: (list: T[]) => T[]) {
  const prev = qc.getQueryData<T[]>(key);
  if (prev) qc.setQueryData<T[]>(key, fn(prev));
  return prev;
}

/** Optimistic enable/disable mutation shared by skills/plugins/rules tabs. */
function useToggle<T extends { id: string; enabled?: boolean }>({
  qc,
  key,
  mutate,
  toast,
}: {
  qc: ReturnType<typeof useQueryClient>;
  key: unknown[];
  mutate: (id: string, enabled: boolean) => Promise<unknown>;
  toast: ToastFn;
}) {
  return useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => mutate(id, enabled),
    onMutate: async ({ id, enabled }) => {
      await qc.cancelQueries({ queryKey: key });
      const prev = optimistic<T>(qc, key, (list) => list.map((it) => (it.id === id ? { ...it, enabled } : it)));
      return { prev };
    },
    onError: (_e, _v, ctx) => {
      if (ctx?.prev) qc.setQueryData(key, ctx.prev);
      toast("Couldn't reach the backend.", "alert");
    },
    onSuccess: () => toast("Saved.", "check"),
  });
}
