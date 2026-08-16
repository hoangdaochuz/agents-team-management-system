# Design

> **STATUS: SUPERSEDED IN PART by `openspec/changes/event-driven-microservices-backend/`**
> (and `AGENTS.md`). This document is the original monolith design. Sections below that no
> longer hold — §1 Components, §6 Realtime, §7 Deployment, §8 ADR-09 — are annotated
> inline. The microservices backend is the implementation of record: one service per domain,
> Kafka event bus (partitioned by `task_id`), Gateway BFF, multi-tenant Auth/Orgs/Resources/
> Admin plane. The per-task sandbox container model in §3–§5 remains the target design for
> the Runner's execution surface (currently a simulated driver; Docker driver pending).
 — AI Agent Kanban System (MVP)

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
- **Go backend** — originally a single binary; now an **event-driven microservices backend**:
  11 Go services under `backend/services/` (gateway, project, task, agent, catalog, settings,
  runner, auth, orgs, resources, admin), one logical Postgres database each, communicating via
  Kafka (choreography, task_id-partitioned topics; task service is the saga coordinator).
- **Gateway (BFF)** — sole HTTP entrypoint on :8080; path-aware reverse proxy
  (`/api/<domain>/...` → owning service), session composition, workspace-context header
  injection (`X-Workspace-ID`/`X-Workspace-IDs`), SSE fan-out for `/tasks/:id/stream`.
- **React SPA** — Vite + TypeScript; served separately (static bundle), talks only to the
  Gateway.
- **Postgres** — one server, 10 logical databases (`deploy/postgres/01-create-databases.sql`),
  each service owns its schema + embedded migrations.
- **Kafka** — KRaft single broker in compose; events in `backend/internal/contracts/events.go`.
- **Task containers** — ephemeral, one per active agent run; build/test sandbox only
  (target design; Runner currently ships a simulated driver — see 5.1).
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
- Steps are persisted to PG by the Runner and published to Kafka `step.*` topics (partitioned
  by `task_id`; `step.id` is the dedup key).
- `GET /api/tasks/:id/stream` is served by the Gateway: it replays persisted steps from the
  Runner (`GET /internal/tasks/{id}/steps`, seq order), then tails the Kafka topic with
  dedup-by-`step.id` and 15s keepalive pings. Browser `EventSource` reconnects natively on
  drop; the replay+dedup makes reconnect resume cleanly.
- *(Superseded from: in-process pub/sub channel keyed by `task_id`.)*

## 7. Deployment (docker-compose)
- `deploy/docker-compose.yml` runs Postgres (10 logical DBs) + Kafka (KRaft) + the 11 service
  containers (per-service image via `deploy/service.Dockerfile`, `ARG SERVICE`) + the Gateway
  on :8080. The SPA is served separately (`make web-dev` or any static host).
- Upstreams are wired via `UPSTREAM_*` env vars on the gateway; Settings holds the master key
  (`SETTINGS_MASTER_KEY`) and serves provider keys only over the internal token/mTLS path.
- *(Superseded from: single `app` container owning the loop + Docker socket + task containers.)*
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
- **ADR-09 Single operator, no auth; one encrypted key set.** Deferred auth to v2 —
  **superseded in phase 10–13 of the microservices migration**: Auth/Orgs/Resources/Admin
  services add signup/approval, sessions, workspace tenancy (`X-Workspace-*` scoping),
  and superadmin gating. Single-operator "no auth" wording no longer holds; the SPA still
  boots from a dev-fallback session until a real signup is completed.
