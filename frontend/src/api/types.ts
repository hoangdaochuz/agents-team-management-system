// Full domain model, derived from docs/spec.md and docs/design.md.
// The frontend declares this contract; the backend implements it incrementally.

export type ISOTime = string; // RFC3339
export type ID = string; // UUID
export type RepoType = "path" | "url";

export type TaskStatus =
  | "backlog"
  | "doing"
  | "review"
  | "done"
  | "blocked"
  | "cancelled"
  | "stopped";

export type TaskType = "task" | "story" | "bug" | "epic";
export type Priority = "highest" | "high" | "medium" | "low";

export type RunRole = "implementer" | "reviewer";
export type RunStatus = "running" | "done" | "aborted" | "stopped";

export type StepKind = "message" | "tool_call" | "tool_result" | "reasoning";
export type Severity = "info" | "warning" | "error" | "critical";

export type Provider = "openai" | "anthropic" | "gemini" | "glm";

// ── Projects ───────────────────────────────────────────────────────
export interface Project {
  id: ID;
  name: string;
  repo_source: string;
  repo_type: RepoType;
  cloned_path: string;
  default_branch: string;
  created_at: ISOTime;
}

// ── Tasks ──────────────────────────────────────────────────────────
// The kanban card / detail view carries UI-friendly fields beyond the core
// entity (type, priority, labels, points, due, progress). The backend may
// persist these in a metadata column or derive them; the API exposes them as a
// single Task resource either way.
export interface Task {
  id: ID;
  project_id: ID;
  agent_id?: ID | null;
  model_override?: string | null;
  title: string;
  prompt: string;
  description?: string;
  status: TaskStatus;
  type?: TaskType;
  priority?: Priority;
  labels?: string[];
  points?: number;
  due_at?: ISOTime | null;
  progress?: number; // 0..100
  branch_name?: string;
  worktree_path?: string;
  comments_count?: number;
  attachments_count?: number;
  created_at: ISOTime;
  updated_at: ISOTime;
}

// ── Agents ─────────────────────────────────────────────────────────
export interface Agent {
  id: ID;
  name: string;
  role: string;
  system_prompt: string;
  default_model: string;
  allowed_tools: string[];
  status?: "running" | "paused" | "idle";
  load?: number; // 0..100
  current_task_id?: ID | null;
  capabilities?: string[]; // skill/tag labels for display
  skill_ids?: ID[];
  mcp_ids?: ID[];
  created_at: ISOTime;
}

// ── Skills ─────────────────────────────────────────────────────────
export interface Skill {
  id: ID;
  name: string;
  description: string;
  body_md: string;
  resources_path?: string;
  created_at: ISOTime;
}

// ── MCP servers ────────────────────────────────────────────────────
export interface McpServer {
  id: ID;
  name: string;
  command: string;
  args: string[];
  env: Record<string, string>;
  created_at: ISOTime;
}

// ── Provider keys (ciphertext never leaves the server) ─────────────
export interface ProviderKey {
  provider: Provider;
  created_at: ISOTime;
}

// ── Runs ───────────────────────────────────────────────────────────
export interface Run {
  id: ID;
  task_id: ID;
  role: RunRole;
  agent_id: ID;
  model: string;
  status: RunStatus;
  round_no: number;
  started_at: ISOTime;
  ended_at?: ISOTime | null;
  token_usage: number;
  error?: string | null;
}

// ── Steps (agent execution events) ─────────────────────────────────
export interface Step {
  id: ID;
  run_id: ID;
  seq: number;
  kind: StepKind;
  payload: StepPayload;
  created_at: ISOTime;
}

export type StepPayload =
  | { content: string } // message | reasoning
  | { tool: string; args: unknown } // tool_call
  | { tool: string; result: unknown }; // tool_result

// ── Findings (reviewer feedback) ───────────────────────────────────
export interface Finding {
  id: ID;
  run_id: ID;
  file: string;
  line: number;
  severity: Severity;
  issue: string;
  recommendation: string;
  status: "open" | "resolved";
}

// ── Feedback (human comments on a task) ────────────────────────────
export interface Feedback {
  id: ID;
  task_id: ID;
  author: "user";
  body: string;
  created_at: ISOTime;
}

// ── Artifacts (derived from a run's patches / documents) ───────────
export interface Artifact {
  id: ID;
  task_id: ID;
  run_id?: ID;
  filename: string;
  kind: "patch" | "document";
  summary: string;
  additions?: number;
  deletions?: number;
  created_at: ISOTime;
}

// ── Activity (dashboard feed; derived from runs + feedback) ────────
export interface ActivityItem {
  id: ID;
  agent_id?: ID;
  agent_name?: string;
  action: string;
  task_id?: ID;
  task_title?: string;
  created_at: ISOTime;
}
