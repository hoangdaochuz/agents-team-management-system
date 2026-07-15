import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AsyncBoundary,
  Button,
  Card,
  CardHead,
  Field,
  Input,
  Segmented,
  Select,
  Switch,
  useToast,
} from "../components/ui";
import { Icon } from "../lib/icons";
import { relativeTime } from "../lib/format";
import { mcpServers, providerKeys } from "../api/client";
import type { Provider } from "../api/types";

type Section = "workspace" | "agent" | "feedback" | "notify" | "integrations";

const NAV: { key: Section; label: string }[] = [
  { key: "workspace", label: "Workspace" },
  { key: "agent", label: "Agent defaults" },
  { key: "feedback", label: "Feedback & pausing" },
  { key: "notify", label: "Notifications" },
  { key: "integrations", label: "Integrations" },
];

const PROVIDERS: { id: Provider; label: string }[] = [
  { id: "openai", label: "OpenAI" },
  { id: "anthropic", label: "Anthropic" },
  { id: "gemini", label: "Google Gemini" },
  { id: "glm", label: "GLM (Zhipu)" },
];

export function SettingsPage() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [section, setSection] = useState<Section>("workspace");

  // ── Workspace ──
  const [wsName, setWsName] = useState("Agent Ops — Dang Anh");
  const [wsTz, setWsTz] = useState("Asia/Ho Chi Minh (UTC+7)");
  const [repoPath, setRepoPath] = useState(
    "github.com/hoangdaochuz/agents-team-management-system",
  );

  // ── Agent defaults ──
  const [model, setModel] = useState("claude-sonnet-5");
  const [maxConcurrent, setMaxConcurrent] = useState("5");
  const [autonomy, setAutonomy] = useState<"assigned" | "matching" | "auto">("matching");

  // ── Feedback & pausing ──
  const [midRunPause, setMidRunPause] = useState(true);
  const [autoPauseOnFail, setAutoPauseOnFail] = useState(true);
  const [requireReview, setRequireReview] = useState(true);
  const [selfFix, setSelfFix] = useState(false);

  // ── Notifications ──
  const [notifPause, setNotifPause] = useState(true);
  const [notifDone, setNotifDone] = useState(true);
  const [notifFail, setNotifFail] = useState(true);
  const [notifDaily, setNotifDaily] = useState(false);

  function handleSave() {
    // Persistence is deferred until the backend exposes a /settings endpoint.
    // We invalidate queries so any future-backed state would refresh.
    toast("Settings saved");
    qc.invalidateQueries({ queryKey: ["settings"] });
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1 className="page-title">Settings</h1>
          <div className="page-sub">Workspace, agent defaults, and notifications</div>
        </div>
        <Button variant="primary" icon={<Icon name="check" size={16} />} onClick={handleSave}>
          Save changes
        </Button>
      </div>

      <div
        className="grid mt-6"
        style={{ gridTemplateColumns: "220px 1fr", alignItems: "start" }}
      >
        {/* Settings nav */}
        <nav
          className="stack"
          style={{ gap: 2, position: "sticky", top: 80, alignSelf: "start" }}
        >
          {NAV.map((n) => (
            <button
              key={n.key}
              onClick={() => setSection(n.key)}
              style={{
                padding: "9px 12px",
                borderRadius: "var(--radius-md)",
                display: "block",
                width: "100%",
                textAlign: "left",
                fontSize: 14,
                background:
                  section === n.key ? "var(--surface)" : "transparent",
                color: section === n.key ? "var(--fg)" : "var(--fg-2)",
                border: "none",
                cursor: "pointer",
                fontWeight: section === n.key ? 500 : 400,
              }}
            >
              {n.label}
            </button>
          ))}
        </nav>

        {/* Active section */}
        <div className="stack">
          {section === "workspace" && (
            <Card>
              <CardHead title="Workspace" />
              <div className="grid" style={{ gridTemplateColumns: "1fr 1fr" }}>
                <Field label="Workspace name">
                  <Input value={wsName} onChange={(e) => setWsName(e.target.value)} />
                </Field>
                <Field label="Time zone">
                  <Select value={wsTz} onChange={(e) => setWsTz(e.target.value)}>
                    <option>Asia/Ho Chi Minh (UTC+7)</option>
                    <option>UTC</option>
                    <option>America/New_York (UTC-5)</option>
                    <option>Europe/London (UTC+0)</option>
                  </Select>
                </Field>
              </div>
              <div className="field mt-4">
                <Field label="Default repo path">
                  <Input
                    className="t-mono"
                    value={repoPath}
                    onChange={(e) => setRepoPath(e.target.value)}
                  />
                </Field>
              </div>
            </Card>
          )}

          {section === "agent" && (
            <Card>
              <CardHead title="Agent defaults" />
              <div className="grid" style={{ gridTemplateColumns: "1fr 1fr" }}>
                <Field label="Model">
                  <Select value={model} onChange={(e) => setModel(e.target.value)}>
                    <option>claude-sonnet-5</option>
                    <option>claude-opus-4-8</option>
                    <option>gpt-4o</option>
                    <option>gemini-2-pro</option>
                    <option>glm-4.6</option>
                  </Select>
                </Field>
                <Field label="Max concurrent agents">
                  <Input
                    type="number"
                    value={maxConcurrent}
                    onChange={(e) => setMaxConcurrent(e.target.value)}
                  />
                </Field>
              </div>
              <div className="field mt-4">
                <Field label="Default autonomy">
                  <Segmented
                    value={autonomy}
                    onChange={(v) => setAutonomy(v)}
                    options={[
                      { value: "assigned", label: "Assigned tasks only" },
                      { value: "matching", label: "Picks matching tasks" },
                      { value: "auto", label: "Fully autonomous" },
                    ]}
                  />
                </Field>
              </div>
            </Card>
          )}

          {section === "feedback" && (
            <Card>
              <CardHead title="Feedback & pausing" />
              <div className="stack" style={{ gap: "var(--space-4)" }}>
                <ToggleRow
                  title="Allow mid-run pausing"
                  hint="Users can pause an agent to give feedback while it's running"
                  checked={midRunPause}
                  onChange={setMidRunPause}
                />
                <ToggleRow
                  title="Auto-pause when tests fail"
                  hint="Agent stops and asks instead of pushing through"
                  checked={autoPauseOnFail}
                  onChange={setAutoPauseOnFail}
                />
                <ToggleRow
                  title="Require review after completion"
                  hint="Done tasks need your sign-off before they're closed"
                  checked={requireReview}
                  onChange={setRequireReview}
                />
                <ToggleRow
                  title="Let agents self-fix and re-run from feedback"
                  hint="Without you clicking each time"
                  checked={selfFix}
                  onChange={setSelfFix}
                />
              </div>
            </Card>
          )}

          {section === "notify" && (
            <Card>
              <CardHead title="Notifications" />
              <div className="stack" style={{ gap: "var(--space-4)" }}>
                <ToggleRow title="When an agent pauses to ask" checked={notifPause} onChange={setNotifPause} />
                <ToggleRow title="When a task is done / up for review" checked={notifDone} onChange={setNotifDone} />
                <ToggleRow title="When a run fails" checked={notifFail} onChange={setNotifFail} />
                <ToggleRow title="Daily summary email" checked={notifDaily} onChange={setNotifDaily} />
              </div>
            </Card>
          )}

          {section === "integrations" && <IntegrationsSection />}
        </div>
      </div>
    </>
  );
}

// ── Toggle row ───────────────────────────────────────────────────────
function ToggleRow({
  title,
  hint,
  checked,
  onChange,
}: {
  title: string;
  hint?: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <label className="spread" style={{ gap: "var(--space-4)" }}>
      <span>
        <div style={{ fontSize: 14, fontWeight: 500 }}>{title}</div>
        {hint && <div className="muted" style={{ fontSize: 12 }}>{hint}</div>}
      </span>
      <Switch checked={checked} onChange={onChange} label={title} />
    </label>
  );
}

// ── Integrations ─────────────────────────────────────────────────────
function IntegrationsSection() {
  const { toast } = useToast();
  const qc = useQueryClient();

  const keysQuery = useQuery({
    queryKey: ["provider-keys"],
    queryFn: () => providerKeys.listProviderKeys(),
  });

  const mcpQuery = useQuery({
    queryKey: ["mcp-servers"],
    queryFn: () => mcpServers.listMcpServers(),
  });

  const connected = new Set((keysQuery.data ?? []).map((k) => k.provider));

  const saveKey = useMutation({
    mutationFn: ({ provider, key }: { provider: Provider; key: string }) =>
      providerKeys.setProviderKey(provider, key),
    onSuccess: () => {
      toast("API key saved");
      qc.invalidateQueries({ queryKey: ["provider-keys"] });
    },
    onError: () => toast("Couldn't save key", "alert"),
  });

  const deleteKey = useMutation({
    mutationFn: (provider: Provider) => providerKeys.deleteProviderKey(provider),
    onSuccess: () => {
      toast("Key removed");
      qc.invalidateQueries({ queryKey: ["provider-keys"] });
    },
    onError: () => toast("Couldn't remove key", "alert"),
  });

  const addMcp = useMutation({
    mutationFn: (input: { name: string; command: string; args?: string[] }) =>
      mcpServers.createMcpServer(input),
    onSuccess: () => {
      toast("MCP server added");
      qc.invalidateQueries({ queryKey: ["mcp-servers"] });
    },
    onError: () => toast("Couldn't add MCP server", "alert"),
  });

  return (
    <>
      <Card>
        <CardHead title="Integrations & API" />
        <AsyncBoundary
          isLoading={keysQuery.isLoading}
          isError={keysQuery.isError}
          error={keysQuery.error}
          data={PROVIDERS}
        >
          {(providers) => (
            <div className="stack" style={{ gap: "var(--space-3)" }}>
              {providers.map((p) => (
                <ProviderKeyRow
                  key={p.id}
                  provider={p.id}
                  label={p.label}
                  connected={connected.has(p.id)}
                  saving={saveKey.isPending}
                  onSave={(key) => saveKey.mutate({ provider: p.id, key })}
                  onRemove={() => deleteKey.mutate(p.id)}
                />
              ))}
            </div>
          )}
        </AsyncBoundary>
      </Card>

      <Card className="mt-4">
        <CardHead title="MCP servers" />
        <AsyncBoundary
          isLoading={mcpQuery.isLoading}
          isError={mcpQuery.isError}
          error={mcpQuery.error}
          data={mcpQuery.data}
          isEmpty={(d) => (d ?? []).length === 0}
          emptyTitle="No MCP servers"
          emptyHint="Add a server to give your agents extra tools."
        >
          {(servers) => (
            <div className="stack" style={{ gap: "var(--space-2)" }}>
              {servers.map((s) => (
                <div key={s.id} className="spread">
                  <div className="row" style={{ alignItems: "center", gap: 12 }}>
                    <span className="avatar sm">MC</span>
                    <div>
                      <div style={{ fontWeight: 600, fontSize: 14 }}>{s.name}</div>
                      <div className="mono muted" style={{ fontSize: 12 }}>
                        {s.command} {s.args.join(" ")}
                      </div>
                    </div>
                  </div>
                  <span className="muted" style={{ fontSize: 12 }}>
                    {relativeTime(s.created_at)}
                  </span>
                </div>
              ))}
            </div>
          )}
        </AsyncBoundary>
        <AddMcpRow onAdd={(input) => addMcp.mutate(input)} saving={addMcp.isPending} />
      </Card>
    </>
  );
}

function ProviderKeyRow({
  provider: _provider,
  label,
  connected,
  saving,
  onSave,
  onRemove,
}: {
  provider: Provider;
  label: string;
  connected: boolean;
  saving: boolean;
  onSave: (key: string) => void;
  onRemove: () => void;
}) {
  const [key, setKey] = useState("");
  return (
    <div className="spread" style={{ gap: "var(--space-4)", alignItems: "flex-end" }}>
      <div className="row" style={{ alignItems: "center", gap: 12, flex: 1 }}>
        <span className="avatar sm">{label.slice(0, 2).toUpperCase()}</span>
        <div style={{ flex: 1 }}>
          <div style={{ fontWeight: 600, fontSize: 14 }}>{label}</div>
          <div className="mono muted" style={{ fontSize: 12 }}>
            {connected ? "key on file" : "not connected"}
          </div>
        </div>
      </div>
      <div className="row" style={{ gap: "var(--space-2)", alignItems: "flex-end" }}>
        <Field label="API key">
          <Input
            type="password"
            placeholder={connected ? "••••••••••••" : "paste key"}
            value={key}
            onChange={(e) => setKey(e.target.value)}
            style={{ minWidth: 220 }}
          />
        </Field>
        <Button
          variant="soft"
          size="sm"
          disabled={!key || saving}
          onClick={() => {
            onSave(key);
            setKey("");
          }}
        >
          Save
        </Button>
        {connected && (
          <Button variant="ghost" size="sm" onClick={onRemove}>
            Remove
          </Button>
        )}
      </div>
    </div>
  );
}

function AddMcpRow({
  onAdd,
  saving,
}: {
  onAdd: (input: { name: string; command: string; args?: string[] }) => void;
  saving: boolean;
}) {
  const [name, setName] = useState("");
  const [command, setCommand] = useState("");
  const [args, setArgs] = useState("");

  return (
    <div className="grid mt-4" style={{ gridTemplateColumns: "1fr 1fr 1fr auto", alignItems: "flex-end" }}>
      <Field label="Name">
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="filesystem" />
      </Field>
      <Field label="Command">
        <Input
          className="t-mono"
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          placeholder="npx"
        />
      </Field>
      <Field label="Args (space-separated)">
        <Input
          className="t-mono"
          value={args}
          onChange={(e) => setArgs(e.target.value)}
          placeholder="-y @modelcontextprotocol/server-filesystem"
        />
      </Field>
      <Button
        variant="soft"
        icon={<Icon name="plus" size={16} />}
        disabled={!name || !command || saving}
        onClick={() => {
          onAdd({
            name,
            command,
            args: args.trim() ? args.trim().split(/\s+/) : undefined,
          });
          setName("");
          setCommand("");
          setArgs("");
        }}
      >
        Add
      </Button>
    </div>
  );
}
