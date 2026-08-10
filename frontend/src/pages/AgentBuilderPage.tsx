import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import { agents } from "../api/client";
import type { Agent, AutonomyMode, Guardrails, Provider } from "../api/types";
import { useAuth } from "../store/auth";
import { Field, Input, Select, Switch, Textarea, useToast } from "../components/ui";
import { Icon } from "../lib/icons";

/** Form state for the builder (flattened from Agent + builder-only fields). */
interface BuilderState {
  name: string;
  role_title: string;
  provider: Provider;
  default_model: string;
  temperature: number;
  max_output_tokens: number;
  system_prompt: string;
  user_prompt_template: string;
  autonomy_mode: AutonomyMode;
  skill_ids: string[];
  allowed_tools: string[];
  mcp_ids: string[];
  knowledge_source_ids: string[];
  guardrails: Guardrails;
}

const BUILTIN_TOOLS = ["read_file", "write_file", "list_files", "run_command", "web_search", "gh_pr_create"];
const SKILL_OPTIONS = ["git-pr-flow", "test-driven", "code-review-strict", "db-migration-safe", "accessibility-audit"];
const MCP_OPTIONS = ["filesystem", "postgres", "playwright"];
const KB_OPTIONS = ["docs/architecture.md", "ADR log", "api.openai.com/docs", "onboarding-handbook.pdf"];
const PROMPT_VARS = ["{{task.title}}", "{{repo}}", "{{skills}}", "{{task.description}}"];

const MODELS_BY_PROVIDER: Record<Provider, string[]> = {
  anthropic: ["claude-sonnet-5", "claude-opus-4-8", "claude-haiku-4-5"],
  openai: ["gpt-4o", "gpt-4o-mini", "o3-mini"],
  gemini: ["gemini-2-pro", "gemini-2-flash"],
  glm: ["glm-4.6", "glm-4.5-air"],
};

const DEFAULT_STATE: BuilderState = {
  name: "",
  role_title: "",
  provider: "anthropic",
  default_model: "claude-sonnet-5",
  temperature: 0.3,
  max_output_tokens: 8192,
  system_prompt: "",
  user_prompt_template: "",
  autonomy_mode: "matching",
  skill_ids: ["git-pr-flow", "test-driven", "accessibility-audit"],
  allowed_tools: ["read_file", "write_file", "list_files", "run_command"],
  mcp_ids: ["filesystem"],
  knowledge_source_ids: ["docs/architecture.md", "ADR log", "onboarding-handbook.pdf"],
  guardrails: {
    auto_pause_on_test_fail: true,
    allow_direct_commits: false,
    allow_shell_commands: true,
    require_approval_before_merge: true,
    max_steps_per_run: 50,
    wall_clock_cap_min: 30,
  },
};

export function AgentBuilderPage() {
  const { id } = useParams();
  const isEdit = Boolean(id);
  const navigate = useNavigate();
  const activeWorkspace = useAuth((s) => s.activeWorkspace);
  const { toast } = useToast();
  const [state, setState] = useState<BuilderState>(DEFAULT_STATE);
  const [draftSaved, setDraftSaved] = useState(false);

  const { data: existing } = useQuery({
    queryKey: ["agent", id],
    queryFn: () => agents.getAgent(id!),
    enabled: isEdit,
  });

  useEffect(() => {
    if (existing) setState(fromAgent(existing));
  }, [existing]);

  const set = <K extends keyof BuilderState>(key: K, value: BuilderState[K]) =>
    setState((s) => ({ ...s, [key]: value }));

  const setGuard = <K extends keyof Guardrails>(key: K, value: Guardrails[K]) =>
    setState((s) => ({ ...s, guardrails: { ...s.guardrails, [key]: value } }));

  const toggleIn = (key: "skill_ids" | "allowed_tools" | "mcp_ids" | "knowledge_source_ids", value: string) =>
    setState((s) => {
      const list = s[key];
      return { ...s, [key]: list.includes(value) ? list.filter((v) => v !== value) : [...list, value] };
    });

  const payload = toPayload(state);

  const mutation = useMutation({
    mutationFn: () => (isEdit ? agents.updateAgent(id!, payload) : agents.createAgent(payload)),
    onSuccess: () => {
      toast(`Agent "${state.name}" ${isEdit ? "updated" : "created"} in ${activeWorkspace?.name ?? "workspace"}`, "check");
      navigate("/agents");
    },
    onError: () => toast("Couldn't reach the backend. Endpoints aren't implemented yet.", "alert"),
  });

  return (
    <>
      <div className="crumbs">
        <Link to="/agents">Agents</Link>
        <span className="sep">/</span>
        <span>{isEdit ? "Edit agent" : "Build an agent"}</span>
      </div>

      <div className="page-head">
        <div>
          <h1 className="page-title">{isEdit ? "Edit agent" : "Build an agent"}</h1>
          <div className="page-sub">Configure identity, model, prompts, autonomy, skills, and guardrails</div>
        </div>
        <div className="row" style={{ gap: "var(--space-2)" }}>
          <Link to="/agents" className="btn btn-ghost">
            Cancel
          </Link>
        </div>
      </div>

      <div className="builder grid" style={{ gridTemplateColumns: "1fr 320px", gap: "var(--space-5)", alignItems: "start" }}>
        <div className="builder-main stack">
          <Section title="Identity">
            <div className="grid form-grid">
              <Field label="Agent name">
                <Input value={state.name} onChange={(e) => set("name", e.target.value)} placeholder="e.g. Pixel" />
              </Field>
              <Field label="Role title">
                <Input value={state.role_title} onChange={(e) => set("role_title", e.target.value)} placeholder="e.g. Frontend / React engineer" />
              </Field>
            </div>
          </Section>

          <Section title="Model">
            <div className="grid form-grid">
              <Field label="Provider">
                <Select value={state.provider} onChange={(e) => set("provider", e.target.value as Provider)}>
                  <option value="anthropic">Anthropic</option>
                  <option value="openai">OpenAI</option>
                  <option value="gemini">Google Gemini</option>
                  <option value="glm">GLM (z.ai)</option>
                </Select>
              </Field>
              <Field label="Model">
                <Select value={state.default_model} onChange={(e) => set("default_model", e.target.value)}>
                  {MODELS_BY_PROVIDER[state.provider].map((m) => (
                    <option key={m} value={m}>
                      {m}
                    </option>
                  ))}
                </Select>
              </Field>
            </div>
            <div className="slider-row">
              <Field label="Temperature">
                <input
                  type="range"
                  min={0}
                  max={100}
                  value={Math.round(state.temperature * 100)}
                  onChange={(e) => set("temperature", Number(e.target.value) / 100)}
                  style={{ width: "100%" }}
                />
              </Field>
              <span className="slider-val mono">{state.temperature.toFixed(2)}</span>
            </div>
            <Field label="Max output tokens">
              <Input
                type="number"
                value={state.max_output_tokens}
                onChange={(e) => set("max_output_tokens", Number(e.target.value))}
              />
            </Field>
          </Section>

          <Section title="Prompts">
            <PromptEditor
              label="System prompt"
              value={state.system_prompt}
              onChange={(v) => set("system_prompt", v)}
              placeholder="You are Pixel, a senior frontend engineer…"
            />
            <PromptEditor
              label="User prompt template"
              value={state.user_prompt_template}
              onChange={(v) => set("user_prompt_template", v)}
              placeholder="Complete: {{task.title}}…"
            />
          </Section>

          <Section title="Mode &amp; autonomy">
            <div className="choice-grid">
              {(
                [
                  ["assigned", "Assigned only", "Works only on tasks explicitly assigned to it."],
                  ["matching", "Picks matching tasks", "Claims tasks whose labels match its capabilities."],
                  ["autonomous", "Fully autonomous", "Finds and works on tasks without assignment."],
                ] as const
              ).map(([mode, label, desc]) => (
                <label key={mode} className={`choice ${state.autonomy_mode === mode ? "on" : ""}`}>
                  <input
                    type="radio"
                    name="autonomy"
                    checked={state.autonomy_mode === mode}
                    onChange={() => set("autonomy_mode", mode as AutonomyMode)}
                  />
                  <div>
                    <b>{label}</b>
                    <div className="muted" style={{ fontSize: 12 }}>{desc}</div>
                  </div>
                </label>
              ))}
            </div>
          </Section>

          <Section title="Skills">
            <ChipPicker options={SKILL_OPTIONS} selected={state.skill_ids} onToggle={(v) => toggleIn("skill_ids", v)} addLabel="+ add skill" />
          </Section>

          <Section title="Tools &amp; MCP">
            <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-2)", marginBottom: "var(--space-3)" }}>
              {BUILTIN_TOOLS.map((t) => (
                <Chip key={t} active={state.allowed_tools.includes(t)} onClick={() => toggleIn("allowed_tools", t)}>
                  {t}
                </Chip>
              ))}
            </div>
            <div className="muted sec-sub">MCP servers</div>
            <ChipPicker options={MCP_OPTIONS} selected={state.mcp_ids} onToggle={(v) => toggleIn("mcp_ids", v)} />
          </Section>

          <Section title="Rules &amp; guardrails">
            <GuardToggle label="Auto-pause when tests fail" checked={Boolean(state.guardrails.auto_pause_on_test_fail)} onChange={(v) => setGuard("auto_pause_on_test_fail", v)} />
            <GuardToggle label="Allow direct commits" checked={Boolean(state.guardrails.allow_direct_commits)} onChange={(v) => setGuard("allow_direct_commits", v)} />
            <GuardToggle label="Allow running shell commands" checked={Boolean(state.guardrails.allow_shell_commands)} onChange={(v) => setGuard("allow_shell_commands", v)} />
            <GuardToggle label="Require human approval before merge" checked={Boolean(state.guardrails.require_approval_before_merge)} onChange={(v) => setGuard("require_approval_before_merge", v)} />
            <div className="grid form-grid" style={{ marginTop: "var(--space-3)" }}>
              <Field label="Max steps per run">
                <Input type="number" value={state.guardrails.max_steps_per_run ?? 50} onChange={(e) => setGuard("max_steps_per_run", Number(e.target.value))} />
              </Field>
              <Field label="Wall-clock cap (min)">
                <Input type="number" value={state.guardrails.wall_clock_cap_min ?? 30} onChange={(e) => setGuard("wall_clock_cap_min", Number(e.target.value))} />
              </Field>
            </div>
          </Section>

          <Section title="Knowledge base access">
            <ChipPicker options={KB_OPTIONS} selected={state.knowledge_source_ids} onToggle={(v) => toggleIn("knowledge_source_ids", v)} />
          </Section>
        </div>

        {/* Sticky summary */}
        <aside className="builder-side">
          <div className="card sticky-save">
            <div className="card-head">
              <h3 className="card-title">Summary</h3>
            </div>
            <SummaryRow label="Workspace" value={activeWorkspace?.name ?? "—"} />
            <SummaryRow label="Model" value={`${state.provider} · ${state.default_model}`} />
            <SummaryRow label="Skills" value={String(state.skill_ids.length)} />
            <SummaryRow label="Tools" value={String(state.allowed_tools.length)} />
            <SummaryRow
              label="Autonomy"
              value={state.autonomy_mode === "assigned" ? "Assigned only" : state.autonomy_mode === "matching" ? "Picks matching" : "Fully autonomous"}
            />
            <div className="stack" style={{ marginTop: "var(--space-4)" }}>
              <button type="button" className="btn btn-primary w-full" onClick={() => mutation.mutate()} disabled={!state.name || mutation.isPending}>
                {mutation.isPending ? "Saving…" : isEdit ? "Save changes" : "Create agent"}
              </button>
              <button
                type="button"
                className="btn btn-ghost w-full"
                onClick={() => {
                  setDraftSaved(true);
                  toast("Saved as draft", "check");
                }}
              >
                Save draft
              </button>
              {draftSaved && <div className="muted center" style={{ fontSize: 12 }}>Draft kept in this session.</div>}
            </div>
          </div>
        </aside>
      </div>
    </>
  );
}

/* ── small building blocks ──────────────────────────────────────── */

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="card">
      <div className="card-head">
        <h2 className="card-title" dangerouslySetInnerHTML={{ __html: title }} />
      </div>
      {children}
    </section>
  );
}

function PromptEditor({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  return (
    <Field label={label}>
      <div className="prompt-editor">
        <Textarea rows={5} value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} />
        <div className="prompt-tools">
          {PROMPT_VARS.map((v) => (
            <button key={v} type="button" className="tag" onClick={() => onChange(`${value} ${v}`)}>
              {v}
            </button>
          ))}
        </div>
      </div>
    </Field>
  );
}

function Chip({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button type="button" className={`chip-pick ${active ? "on" : ""}`} onClick={onClick}>
      {children}
    </button>
  );
}

function ChipPicker({
  options,
  selected,
  onToggle,
  addLabel,
}: {
  options: string[];
  selected: string[];
  onToggle: (v: string) => void;
  addLabel?: string;
}) {
  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-2)" }}>
      {options.map((o) => (
        <Chip key={o} active={selected.includes(o)} onClick={() => onToggle(o)}>
          {selected.includes(o) ? <Icon name="check" size={12} /> : null} {o}
        </Chip>
      ))}
      {addLabel && (
        <button type="button" className="chip-pick" onClick={() => undefined} style={{ borderStyle: "dashed" }}>
          {addLabel}
        </button>
      )}
    </div>
  );
}

function GuardToggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="flag-row">
      <span className="flag-name">{label}</span>
      <Switch checked={checked} onChange={onChange} label={label} />
    </div>
  );
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="summary-row spread" style={{ padding: "var(--space-2) 0", borderBottom: "1px solid var(--border-soft)" }}>
      <span className="muted" style={{ fontSize: "var(--text-sm)" }}>{label}</span>
      <span style={{ fontSize: "var(--text-sm)", fontWeight: 600 }}>{value}</span>
    </div>
  );
}

/* ── mappers ────────────────────────────────────────────────────── */
function fromAgent(a: Agent): BuilderState {
  return {
    ...DEFAULT_STATE,
    name: a.name,
    role_title: a.role_title ?? "",
    provider: a.provider ?? DEFAULT_STATE.provider,
    default_model: a.default_model || DEFAULT_STATE.default_model,
    temperature: a.temperature ?? DEFAULT_STATE.temperature,
    max_output_tokens: a.max_output_tokens ?? DEFAULT_STATE.max_output_tokens,
    system_prompt: a.system_prompt ?? "",
    user_prompt_template: a.user_prompt_template ?? "",
    autonomy_mode: a.autonomy_mode ?? DEFAULT_STATE.autonomy_mode,
    skill_ids: a.skill_ids?.map(String) ?? DEFAULT_STATE.skill_ids,
    allowed_tools: a.allowed_tools ?? DEFAULT_STATE.allowed_tools,
    mcp_ids: a.mcp_ids?.map(String) ?? DEFAULT_STATE.mcp_ids,
    knowledge_source_ids: a.knowledge_source_ids?.map(String) ?? DEFAULT_STATE.knowledge_source_ids,
    guardrails: a.guardrails ?? DEFAULT_STATE.guardrails,
  };
}

function toPayload(s: BuilderState) {
  return {
    name: s.name,
    role: s.role_title,
    system_prompt: s.system_prompt,
    default_model: s.default_model,
    allowed_tools: s.allowed_tools,
    role_title: s.role_title,
    provider: s.provider,
    temperature: s.temperature,
    max_output_tokens: s.max_output_tokens,
    autonomy_mode: s.autonomy_mode,
    user_prompt_template: s.user_prompt_template,
    guardrails: s.guardrails,
    knowledge_source_ids: s.knowledge_source_ids,
    skill_ids: s.skill_ids,
    mcp_ids: s.mcp_ids,
  };
}
