# Implementation Tasks — AI Agent Kanban System (MVP)

Ordered by dependency. Each task has a **goal**, **acceptance criteria**, and a **disjoint file
scope** so the work can later be parallelized (e.g. via `parallel-team-executor`). Phases are
sequential; tasks within a phase are mostly parallelizable where scopes don't overlap.

Legend: `[P]` parallelizable within its phase · `[S]` sequential (blocks the phase).

---

## Phase 0 — Bootstrap
**T0.1 [S] Project & tooling scaffold**
- Goal: Go module, directory layout, Dockerfile, docker-compose, Makefile, CI lint+test.
- AC: `cd backend && go build ./...` and `go test ./...` pass on a fresh clone; `docker compose -f deploy/docker-compose.yml up` starts
  `app` + `postgres`; `golangci-lint` clean.
- Scope: `backend/go.mod`, `backend/cmd/server/main.go`, `backend/Dockerfile`, `deploy/docker-compose.yml`, `Makefile`,
  `.github/workflows/ci.yml`, `backend/runner/Dockerfile` (base task image: go+git, no secrets).
- Deps: none.

## Phase 1 — Persistence & domain
**T1.1 [S] Postgres schema + migrations**
- Goal: schema from `design.md` §2 as versioned migrations.
- AC: `migrate up` creates all tables/indices/FKs; down is clean.
- Scope: `backend/internal/store/migrations/*.sql`.
- Deps: T0.1.

**T1.2 [P] Repository layer (pgx/sqlc)**
- Goal: typed repos for projects, tasks, agents, skills, mcp_servers, runs, steps, findings,
  feedback, task_thread, provider_keys.
- AC: CRUD + the queries the services need; unit tests against a real PG (testcontainers).
- Scope: `backend/internal/store/*.go`, generated sqlc in `backend/internal/store/gen`.
- Deps: T1.1.

**T1.3 [P] Crypto/secret store**
- Goal: AES-GCM encrypt/decrypt for provider_keys; envelope key from `ENCRYPTION_KEY` env.
- AC: round-trip test; keys never appear in logs; provider_keys row stores ciphertext only.
- Scope: `backend/internal/secrets/*.go`.
- Deps: T1.1.

## Phase 2 — HTTP API scaffold
**T2.1 [S] Server, routing, middleware, errors**
- Goal: `net/http` (or chi) server with structured logging, request IDs, JSON errors, graceful
  shutdown; embeds SPA static assets.
- AC: health endpoint works; panic→500 recovery; logs JSON.
- Scope: `backend/internal/httpapi/server.go`, `backend/internal/httpapi/middleware/*.go`.
- Deps: T1.2.

**T2.2 [P] REST handlers (CRUD surfaces)**
- Goal: handlers for projects, tasks, agents, skills, mcp_servers; plus task actions
  (move column, re-run, stop, open-PR).
- AC: endpoints match the SPA contract; validation; integration tests.
- Scope: `backend/internal/httpapi/handlers/*.go`.
- Deps: T2.1, T1.2.

**T2.3 [P] SSE stream endpoint**
- Goal: `GET /api/tasks/:id/stream` replaying persisted steps then tailing a pub/sub channel.
- AC: a published step reaches a connected client; reconnect resumes without duplication.
- Scope: `backend/internal/httpapi/stream.go`, `backend/internal/pubsub/*.go`.
- Deps: T2.1.

## Phase 3 — Multi-provider abstraction
**T3.1 [S] Canonical types + Provider interface**
- Goal: `Message`, `Tool`, `ToolCall`, `Response`, `Provider` interface, registry by provider name.
- AC: interface compiles; a fake provider satisfies it; model override plumbed through.
- Scope: `backend/internal/llm/types.go`, `backend/internal/llm/provider.go`.
- Deps: T1.3.

**T3.2 [P] OpenAI adapter (+ GLM via compat base URL)**
- AC: real call returns content + tool_calls; tool result round-trip; GLM endpoint selectable.
- Scope: `backend/internal/llm/openai/*.go`.
- Deps: T3.1.

**T3.3 [P] Anthropic adapter (community SDK)**
- AC: message + tool-use round-trip; isolated behind interface (swap risk documented).
- Scope: `backend/internal/llm/anthropic/*.go`.
- Deps: T3.1.

**T3.4 [P] Gemini adapter (official Go genai SDK)**
- AC: message + function-calling round-trip.
- Scope: `backend/internal/llm/gemini/*.go`.
- Deps: T3.1.

## Phase 4 — Agent loop
**T4.1 [S] Loop engine + tool registry**
- Goal: the loop in `design.md` §3.1; built-in tools `read_file/write_file/list_files/run_command`;
  inject agent system prompt + attached skills; reconstruct context from `task_thread`; emit steps.
- AC: against a fake provider + fake tool dispatcher, a 2-step run produces persisted+published
  steps; caps enforced (steps/tokens/wall-clock) and abort path tested.
- Scope: `backend/internal/agent/loop.go`, `backend/internal/agent/tools/*.go`, `backend/internal/agent/context.go`.
- Deps: T3.1, T1.2.

## Phase 5 — Container execution sandbox
**T5.1 [S] Docker exec sandbox**
- Goal: backend creates/starts a task container (shared runner image, worktree bind-mounted RW,
  no secrets), executes `run_command` via Docker exec API, captures streamed output, tears down on
  run end / abort / Stop.
- AC: a tool `run_command` runs `go test` inside the container; output returned; container holds
  no env secrets (assert in test); killed on context cancel.
- Scope: `backend/internal/sandbox/container.go`.
- Deps: T0.1 (runner image), T4.1 (tool interface).

## Phase 6 — Git operations (backend-owned)
**T6.1 [S] Repo provisioning + worktree manager**
- Goal: register project → clone (URL) or use (path) into managed storage using shared SSH key;
  `git fetch` before branching; `git worktree add` per task on branch `agent/<id>-<slug>`.
- AC: registering a URL repo clones it; a task gets a worktree+branch; fetch picks up new main.
- Scope: `backend/internal/git/repo.go`, `backend/internal/git/worktree.go`.
- Deps: T1.2.

**T6.2 [S] Commit / push / PR**
- Goal: post-run `git add`+`commit` (conventional msg) on task branch; on human action
  `git push` (shared key) + `gh pr create`.
- AC: implementer run leaves a commit on the branch; open-PR action creates a real PR against a
  test repo; never executed inside a container.
- Scope: `backend/internal/git/commit.go`, `backend/internal/git/pr.go`.
- Deps: T6.1.

## Phase 7 — Task runner orchestration
**T7.1 [S] Async runner + lifecycle**
- Goal: queue + worker that, on task→Doing, wires (worktree, container, provider, MCP) and runs
  the implementer loop; on task→Review runs the reviewer loop; implements the implementer↔reviewer
  auto-loop (≤5 rounds) and the Stop/abort transitions.
- AC: full S1 scenario runs end-to-end against a fake provider and a real container; 5-round cap
  surfaces to human; Stop kills the container.
- Scope: `backend/internal/runner/runner.go`, `backend/internal/runner/review_loop.go`.
- Deps: T4.1, T5.1, T6.x, T2.3, T1.2.

**T7.2 [P] Reviewer profile + findings**
- Goal: reviewer agent produces structured verdict+findings; findings persisted; REQUEST_CHANGES
  feeds back into the implementer as task input.
- AC: reviewer emits APPROVE or findings with file/line/issue/recommendation; findings resolve on
  fix.
- Scope: `backend/internal/runner/reviewer.go`, `backend/internal/store/findings.go` (if not in T1.2).
- Deps: T4.1, T7.1.

## Phase 8 — Management layer
**T8.1 [P] Agents CRUD + attachment**
- Goal: create/edit agents (persona, default model, allowed_tools); attach skills & MCP servers.
- AC: persisted; attaching/detaching updates run-time tool+context sets.
- Scope: `backend/internal/mgmt/agents.go` + handler T2.2 wiring.
- Deps: T1.2, T2.2.

**T8.2 [P] Skills import + injection**
- Goal: parse uploaded file/zip into a skill (frontmatter+body+resources); store; inject all
  attached skills into agent context at run time.
- AC: upload a `.zip` skill → stored; an agent with the skill gets its text in the system context
  during a run.
- Scope: `backend/internal/mgmt/skills.go`, `backend/internal/agent/skills.go`.
- Deps: T1.2, T4.1.

**T8.3 [P] MCP client (stdio) + tool bridging**
- Goal: using official Go MCP SDK, spawn the agent's configured stdio servers at run start,
  enumerate tools, bridge each as an LLM tool, forward calls, tear down at run end.
- AC: attach a `filesystem` MCP server to an agent; its tools appear and are callable during a run;
  server process is gone after the run.
- Scope: `backend/internal/mcp/client.go`, `backend/internal/agent/mcp_tools.go`.
- Deps: T4.1, T3.1.

## Phase 9 — Frontend SPA
**T9.1 [S] App shell, routing, API client, SSE hook**
- Goal: Vite+React+TS app; React Query client; SSE hook for live steps.
- AC: boots against the Go API; reconnects SSE on drop.
- Scope: `frontend/src/{app,api,hooks}/*`.
- Deps: T2.x.

**T9.2 [P] Kanban board + task detail**
- Goal: drag-and-drop columns (`@dnd-kit`); task detail with prompt, status, model picker,
  streamed step log, branch diff viewer (`react-diff-viewer`/Monaco), feedback thread, re-run,
  Stop, Open-PR.
- AC: implements scenarios S1–S4 against the backend.
- Scope: `frontend/src/features/{board,task}/*`.
- Deps: T9.1.

**T9.3 [P] Management UI**
- Goal: CRUD pages for projects, agents, skills (upload), MCP servers.
- AC: all CAP-7/8/9 flows usable from the UI.
- Scope: `frontend/src/features/{projects,agents,skills,mcp}/*`.
- Deps: T9.1.

## Phase 10 — Integration & hardening
**T10.1 [S] End-to-end smoke + security verification**
- Goal: scripted E2E against real provider keys (opt-in) + a real GitHub repo; assert: no secrets
  in container env (`docker inspect`), PR created on approve, caps/Stop behave, SSE survives
  refresh.
- AC: S1–S4 pass; security assertions pass.
- Scope: `test/e2e/*`, `docs/runbook.md`.
- Deps: all above.

---

## Dependency summary (critical path)
T0.1 → T1.1 → T1.2/T1.3 → T2.x & T3.x (parallel) → T4.1 → {T5.1, T6.x, T8.x} → T7.1 → T7.2 → T10.1.
Frontend (Phase 9) can proceed in parallel against the API contract from Phase 2.
