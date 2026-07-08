# Design — AI Agent Kanban System (MVP)

Companion to `spec.md`. Captures architecture, data model, the agent execution model, security
model, and the rationale for the load-bearing decisions (ADR-style).

## 1. System overview

```
                       ┌───────────────────────────────┐
   Browser (React SPA) │  Kanban UI · Task detail ·     │
   ───────────────────▶│  Diff viewer · Mgmt (agents/   │── REST ──┐
   ◀─── SSE (steps) ───│  skills/MCP) · Step log        │          │
                       └───────────────────────────────┘          │
                                                                  ▼
        ┌────────────────────────────────────────────────────────────┐
        │                     Go Backend (API + engine)               │
        │                                                             │
        │  HTTP handlers ─▶ Service layer ─▶ Repos (sqlc/pgx) ─▶ PG   │
        │                                                             │
        │  Task runner (async):  Agent loop (tool-using)              │
        │     │                          │                            │
        │     │ LLM calls (host net)     │ tool dispatch              │
        │     ▼                          ▼                            │
        │  Provider adapters          read_file/write_file/list_files │
        │  (OpenAI/Anthropic/         → write to host worktree path   │
        │   Gemini/GLM)               run_command → docker exec ──┐   │
        │                                                          │   │
        │  MCP client (Go SDK): spawns stdio MCP servers on host,  │   │
        │  bridges their tools as LLM tools                        │   │
        │                                                          │   │
        │  Git ops (host): commit / push / gh pr create            │   │
        └──────────────────────────────────────────────────────────┘   │
                              │ Docker socket                          │
                              ▼                                        │
        ┌────────────────────────────────────┐   bind-mount worktree ◀─┘
        │  Per-task container (build/test)     │
        │  — NO API keys, NO git credentials — │
        │  worktree mounted RW                 │
        └────────────────────────────────────┘

        Postgres: projects, tasks, agents, skills, mcp_servers,
                  runs, steps, findings, secrets(encrypted)
```

### Components
- **Go backend** — single binary serving REST + SSE, running the async task runner and the agent
  loop. Owner of: provider calls, MCP client, git operations, container orchestration.
- **React SPA** — Vite + TypeScript; served as static bundle (embedded in the Go binary or
  served separately). State via React Query (server) + Zustand (UI).
- **Postgres** — sole data store.
- **Task containers** — ephemeral, one per active agent run; build/test sandbox only.
- **MCP servers** — external stdio processes spawned by the backend on the host per agent run.

## 2. Data model (Postgres)

```
projects        id, name, repo_source (path|url), repo_type, cloned_path,
                default_branch, created_at
tasks           id, project_id, agent_id, model_override (nullable),
                title, prompt, status (column), branch_name, worktree_path,
                created_at, updated_at
                -- status ∈ backlog|doing|review|done|blocked|cancelled|stopped
agents          id, name, role, system_prompt, default_model (provider+model),
                allowed_tools (jsonb), created_at
agent_skills    agent_id, skill_id        (many-to-many)
agent_mcps      agent_id, mcp_server_id   (many-to-many)
skills          id, name, description, body_md, resources_path, created_at
mcp_servers     id, name, command, args (jsonb), env (jsonb), created_at
runs            id, task_id, role (implementer|reviewer), agent_id, model,
                status (running|done|aborted|stopped), round_no,
                started_at, ended_at, token_usage, error
steps           id, run_id, seq, kind (message|tool_call|tool_result|reasoning),
                payload (jsonb), created_at
findings        id, run_id (reviewer run), file, line, severity, issue,
                recommendation, status (open|resolved)
feedback        id, task_id, author, body, created_at   (conversation thread)
task_thread     id, task_id, role (user|agent|reviewer), content, created_at
                -- normalized thread used to reconstruct agent context
provider_keys   provider (pk), ciphertext, created_at   (encrypted at rest)
```

### Worktree / repo storage (host filesystem, not PG)
- Managed clone root: `/var/lib/aaks/repos/<project_id>/.git` (bare-ish clone owned by backend).
- Per-task worktree: `/var/lib/aaks/worktrees/<task_id>` (a `git worktree add` off the clone),
  bind-mounted RW into the task container.

## 3. Agent execution model (the load-bearing design)

### 3.1 The loop (runs in the backend)
```
build system prompt = agent.system_prompt + injected skills (markdown)
tools = agent.allowed_tools (built-ins) + MCP tools (from agent's MCP servers)
context = task.prompt + task_thread (prior turns) + (reviewer: current findings)
loop:
    resp = provider.Call(model, system, messages, tools)     # host network, keys on host
    if resp has tool_calls:
        for each call:
            result = dispatch(call)                          # see 3.2
            append step(tool_call) + step(tool_result); stream via SSE
        continue
    else:
        append step(message); break
enforce caps: steps ≤ N, tokens ≤ T, wall-clock ≤ 30m; abort on breach
```
- The reviewer variant sets `role=reviewer`, gives the branch diff/test results as context, and
  requires the final message to be a structured verdict (`APPROVE` | `REQUEST_CHANGES` + findings).

### 3.2 Tool dispatch
- `read_file`/`list_files` — backend reads host worktree path (same files the container sees).
- `write_file` — backend writes host worktree path; container sees the change on next exec.
- `run_command` — backend runs the command **inside the task container** via Docker exec API
  (cwd = mounted worktree). Output streamed back as the tool result.
- **MCP tools** — backend forwards the call to the relevant MCP server process (host) and returns
  its response. MCP servers are spawned at run start (stdio), kept alive for the run, torn down
  at run end.

### 3.3 Git (backend-only, host)
After an implementer pass the backend (not the container) does: `git -C <worktree> add` +
`commit` (conventional message) on the task branch. On human "Open PR": `git push` (shared key) +
`gh pr create`. Worktree creation/branching is also backend-side via `git worktree`.

### 3.4 Why the loop lives in the backend (not the container)
Keeps API keys and git credentials **off the execution surface**: the container never needs them.
The container is reduced to a pure, revocable build/test/edit sandbox. This is the core security
property (see §5). The cost is a `docker exec` round-trip per `run_command` — acceptable for an
interactive coding loop.

## 4. Multi-provider abstraction

```
type Provider interface {
    Call(ctx, model, system string, messages []Message, tools []Tool) (Response, error)
}
```
- Internal canonical types: `Message{role, content[]block}`, `Tool{name, schema}`,
  `ToolCall{id, name, args}`, `Response{content, tool_calls, usage}`.
- Implementations: `openaiProvider` (also serves GLM via OpenAI-compatible base URL),
  `anthropicProvider`, `geminiProvider`.
- **Dependency risk:** Anthropic has no official Go SDK; use community `liushuangls/go-anthropic`.
  Isolate it behind the interface so it can be swapped. Gemini via official Go genai SDK; OpenAI
  via official `openai/openai-go`.
- Keys resolved from `provider_keys` (decrypted in-process); never serialized into logs, step
  payloads, or containers.

## 5. Security model
- **Threat:** prompt-injection from repo content or tool output instructing the agent to run
  malicious commands, exfiltrate keys, or damage the host/keys.
- **Controls:**
  1. Agent `run_command` executes only inside a per-task container — no host shell, host FS
     limited to the bind-mounted worktree.
  2. Container holds **no API keys and no git credentials** → even a fully compromised agent
     cannot read keys or push as the system.
  3. Mutating git is backend-only → a compromised agent cannot land code or push without the
     backend's explicit post-run commit + human-gated PR.
  4. Wall-clock/step/token caps bound blast radius; `Stop` kills the container.
- **Residual/trusted surface:** registered MCP servers run on the host and are trusted-by-choice
  (the operator installed them). They are not arbitrary agent code.

## 6. Realtime (SSE)
- Each run writes steps to PG and to an in-process pub/sub channel keyed by `task_id`.
- `GET /api/tasks/:id/stream` is an SSE endpoint that replays persisted steps then tails the
  channel. Browser `EventSource` reconnects natively on drop.

## 7. Deployment (docker-compose)
```
services:
  app:        # Go backend container
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock   # spawn sibling task containers
      - aaks-data:/var/lib/aaks                      # repos + worktrees (shared with task ctrs)
    env: GIT_KEY_FILE, ENCRYPTION_KEY, DB_DSN, GITHUB_TOKEN ...
    depends_on: [postgres]
  postgres:   image: postgres:16, volume for PG data
volumes:
  aaks-data:  # bind-mounted into task containers so backend + container share worktree paths
```
- Task containers are created by the backend (Docker API) with `--mount` of the relevant
  worktree subpath under `aaks-data`, read-write, using a shared base image
  (`aaks-runner: go toolchain + git`, no secrets baked in).

## 8. Decision log (ADRs)

- **ADR-01 Real repo, worktree-per-task.** Honors "affects the real repo" while giving
  concurrency isolation. Rejected: clone-per-task (diverges from real repo); shared tree (corrupts).
- **ADR-02 Go-native agent loop (no framework).** Only way to get uniform multi-model switching
  across the 4 providers; single binary; loop is small. Rejected: spawn-a-CLI (caps model
  choice), Python microservice (extra runtime).
- **ADR-03 Backend owns the loop + git; container is credential-less sandbox.** Maximizes the
  security property (keys/git off the execution surface). Rejected: agent-in-container (re-exposes
  secrets).
- **ADR-04 Container-per-task command execution.** Contains prompt-injection-driven commands.
  Rejected: host allowlist (leaky), bare host (unsafe).
- **ADR-05 PR-only merge, human gate.** Code lands on `main` only via a human-opened PR; never
  auto-merged. Implies every repo needs a GitHub remote.
- **ADR-06 docker-compose over K8s.** K8s reopens the loop-location/credential questions and is
  overkill for a single-operator MVP. Compose keeps the backend-owned-loop model intact.
- **ADR-07 Agent as a bundle; skills & MCP attach to it.** Makes "manage agents/skills/MCP" a
  coherent feature; model picker becomes an agent property (still overridable per task).
- **ADR-08 Skills = markdown modules; MCP = stdio only.** Lightest useful MVP; remote MCP and
  executable skills deferred.
- **ADR-09 Single operator, no auth; one encrypted key set.** Defers the large auth feature to v2.
