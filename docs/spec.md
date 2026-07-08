# Specification — AI Agent Kanban System (MVP)

## 1. Purpose
A single-operator web application that combines a **kanban task board** with an **autonomous AI
agent execution engine** and a **management layer** for agents, skills, and MCP servers. A user
assigns a task (a coding unit from an implementation plan, or a research task) to a configured
agent; the agent works inside the task's repo and produces a branch of real code or a research
artifact; the user monitors progress live, reviews the result, gives feedback, and an
implementer↔reviewer agent loop converges before the human opens a pull request.

## 2. Scope

### In scope (MVP)
- Multi-project registration (local path **or** git URL).
- Kanban board: `Backlog → Doing → Review → Done` (+ `Blocked`/`Cancelled`).
- Task = coding task (real code on a branch) **or** research task (markdown artifact).
- Autonomous agent execution via a Go-native tool-using loop.
- Multi-model: OpenAI, Anthropic, Google Gemini, GLM (Zhipu); model selectable per task.
- Live monitoring of agent steps (tool calls, results, reasoning) over SSE.
- Feedback loop: user reviews code, comments, agent re-works the same branch.
- Reviewer agent + auto-fix loop until APPROVE; human-initiated PR creation.
- Management: Agents (persona/model/skills/MCP/tools), Skills (markdown, UI upload),
  MCP servers (stdio).
- Per-task containerized command execution (isolation).
- Single operator, no auth; encrypted key storage.

### Out of scope (v2+)
- Multi-user / auth / RBAC.
- Multi-credential git (per-project deploy keys).
- Remote (HTTP/SSE) MCP transport; MCP resource/prompt surfacing (tools only in MVP).
- Smart/description-based skill selection (all attached skills injected in MVP).
- Skill directory-sync / registry pull; executable/tool skills.
- Direct-merge for local-only repos (merge is PR-only; GitHub remote required).
- Kubernetes deployment; auto-merge.

## 3. Definitions
- **Project** — a registered repo (local path or git URL) that tasks belong to.
- **Task** — a kanban ticket: a prompt describing coding/research work, with status, assigned
  agent, branch, and a conversation/feedback thread.
- **Agent** — a configurable bundle: name/role, system prompt, default model (overridable per
  task), attached skills, attached MCP servers, allowed built-in tools.
- **Skill** — a markdown instruction module (frontmatter `name`/`description` + body, optional
  resource files) attached to agents and injected into context at run time.
- **MCP server** — a stdio MCP server config (`command`/`args`/`env`) attached to agents; its
  tools are brokered to the LLM by the backend at run time.
- **Agent run** — one execution of the agent loop on a task (implementer pass or reviewer pass).
- **Step** — one event in an agent run (LLM message, tool call, tool result, reasoning).

## 4. Capabilities & Requirements

### CAP-1: Project management
- REQ-1.1 User can register a project by **local path** (use in place) or **git URL** (backend
  clones into managed storage and owns fetches).
- REQ-1.2 A single shared SSH/deploy key (system-level config) authenticates all git operations.
- REQ-1.3 Every registered repo must expose a **GitHub remote** (required for PR-only merge);
  the system validates this on registration.
- REQ-1.4 User can list/open/remove projects.

### CAP-2: Task & kanban
- REQ-2.1 Tasks live in columns: `Backlog`, `Doing`, `Review`, `Done`, plus `Blocked`/`Cancelled`.
- REQ-2.2 A task is assigned to an **agent**; the agent's default model applies unless the user
  overrides the model for that task.
- REQ-2.3 Moving a task to `Doing` creates a **worktree + branch** (`agent/<task-id>-<slug>`)
  off the project repo's default branch and starts an implementer agent run.
- REQ-2.4 Moving to `Review` starts a reviewer agent run.
- REQ-2.5 Tickets can be dragged between columns; agent-event transitions are automatic
  (implementer done ⇒ `Review`; reviewer APPROVE ⇒ staged for merge).
- REQ-2.6 A `Stop` action on a `Doing`/`Review` ticket kills the running container and marks the
  task stopped.

### CAP-3: Agent execution
- REQ-3.1 The agent loop runs **in the backend** as a Go-native tool-using loop.
- REQ-3.2 Built-in tools: `read_file`, `write_file`, `list_files`, `run_command`; toggleable per
  agent.
- REQ-3.3 `run_command`, file edits, and build/test execute **inside the task's container**
  (worktree bind-mounted); the backend drives the container via the Docker exec API.
- REQ-3.4 **No API keys and no git credentials** are present inside any task container.
- REQ-3.5 Every agent run emits a structured **step log** (tool, args, result, reasoning),
  persisted and streamed live over SSE.
- REQ-3.6 Bounded execution: per-run step cap (default ~50 tool calls), per-run token cap,
  per-container wall-clock timeout (default 30 min); on breach the run is aborted and surfaced.

### CAP-4: Multi-model providers
- REQ-4.1 Four providers supported: **OpenAI**, **Anthropic**, **Google Gemini**, **GLM**.
- REQ-4.2 One internal canonical request shape `{messages, tools, system}` is translated per
  provider; tool-call results are translated back.
- REQ-4.3 GLM may be served via the OpenAI-compatible endpoint (reusing the OpenAI adapter).
- REQ-4.4 Model is selectable per task, overriding the assigned agent's default model.
- REQ-4.5 API keys are user-supplied, stored **encrypted at rest**, never logged, never sent to
  any container.

### CAP-5: Feedback loop
- REQ-5.1 On a `Doing` task the user can review the current branch diff and post feedback.
- REQ-5.2 A "Re-run with feedback" action starts a new implementer pass on the **same branch**,
  with feedback injected as additional instruction and the **prior conversation thread** included.
- REQ-5.3 The conversation thread (prompt → output → feedback → output → …) is preserved on the
  ticket.

### CAP-6: Review loop
- REQ-6.1 In `Review`, a **reviewer agent** (distinct profile) inspects the branch: reads files,
  runs tests, diffs against base, and emits a verdict `APPROVE` or `REQUEST_CHANGES` with a list
  of findings (`file`/`line`/`issue`/`recommendation`).
- REQ-6.2 On `REQUEST_CHANGES`, the task auto-returns to `Doing` and the implementer is fed the
  findings; loops until `APPROVE` or the **5-round cap** is hit (then surfaced to the human).
- REQ-6.3 On `APPROVE`, the task is staged for merge; **no automatic PR**.
- REQ-6.4 A human "Open PR" action creates a GitHub PR from the task branch to the default branch
  via `gh`/GitHub API.

### CAP-7: Management — Agents
- REQ-7.1 CRUD for agents: name/role, system prompt, default model, attached skills, attached MCP
  servers, allowed built-in tools.
- REQ-7.2 Two built-in profiles ship by default: **Implementer** and **Reviewer**.

### CAP-8: Management — Skills
- REQ-8.1 A skill is a markdown module: frontmatter (`name`, `description`) + body, optionally
  bundled with resource files.
- REQ-8.2 User imports a skill via **UI upload** of a single file or a `.zip` bundle.
- REQ-8.3 Skills attach to one or more agents; at run time all attached skills are injected into
  the agent context.

### CAP-9: Management — MCP
- REQ-9.1 An MCP server config = name + stdio transport (`command`, `args`, `env`).
- REQ-9.2 MCP servers attach to agents; at run time the backend (MCP client) spawns the agent's
  configured servers and enumerates their tools.
- REQ-9.3 Each MCP tool is bridged as an LLM-callable tool; when the LLM invokes it, the backend
  forwards the call to the MCP server and returns the result.
- REQ-9.4 MCP servers run **on the backend host** (not inside task containers).

### CAP-10: Security & isolation (cross-cutting)
- REQ-10.1 Per-task container is credential-less; only build/test/edit happens there.
- REQ-10.2 Mutating git (commit, push, PR) is performed **only by the backend**.
- REQ-10.3 API keys + git credential stored encrypted at rest; single shared git credential.
- REQ-10.4 Concurrent Doing tasks use disjoint worktrees/branches (no shared working tree writes).

## 5. Non-functional requirements
- **NFR-1 Operability:** single `docker-compose up` brings up app + Postgres + Docker socket
  wiring for task containers.
- **NFR-2 Resilience:** an agent run survives client disconnects; SSE reconnect resumes the live
  view; aborted/Stopped runs leave the task in a recoverable state.
- **NFR-3 Cost safety:** hard caps (rounds, steps, tokens, wall-clock) + Stop prevent runaway
  spend on user-supplied keys.
- **NFR-4 Auditability:** step logs and review findings are persisted and reviewable post-hoc.
- **NFR-5 Maintainability:** provider abstraction and MCP bridging are interface-driven so new
  providers/MCP tools require no core changes.

## 6. Key scenarios

### S1: Implement a feature end-to-end
User registers a GitHub repo project → creates a task "add /healthz endpoint" → assigns the
Implementer agent (model: Claude) → drags to `Doing`. Backend creates worktree+branch, spawns a
task container, runs the agent loop (edits files, runs `go test`) streaming steps over SSE. User
watches, then the agent finishes → task auto-moves to `Review`. Reviewer agent runs, requests
changes → task back to `Doing`, implementer fixes → reviewer APPROVE. User clicks "Open PR".

### S2: Feedback-driven re-work
During `Doing`, user reads the diff, posts "also handle the error from the DB call". Clicks
"Re-run with feedback". Implementer continues on the same branch with the thread; produces an
updated diff.

### S3: Attach a skill + MCP server to an agent
User uploads a `code-review.md` skill, registers a `filesystem` MCP server (stdio), creates an
agent "Senior Reviewer" attaching both, and assigns a task to it. At run time the backend injects
the skill text and bridges the MCP filesystem tools to the reviewer LLM.

### S4: Runaway agent is stopped
An implementer loops editing without converging. The step cap / token cap / 30-min timeout trips,
or the user hits `Stop`; the container is killed, the task is marked stopped with the partial step
log preserved.
