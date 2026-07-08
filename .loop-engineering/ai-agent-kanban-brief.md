# Loop-Engineering Brief: AI Agent Kanban System (MVP)
slug: ai-agent-kanban | branch: (not yet created — greenfield) | started: 2026-07-08 | status: spec

> This project entered via `/loop-engineering` but is **greenfield + full-stack**, which is
> outside loop-engineering's scope (Go-backend-only, no greenfield scaffolding). Per the stack
> gate, the orchestrator stopped and the user chose the **"grill me, then spec it"** path.
> This document is the spec-phase artifact; the implement↔review→PR loop is NOT run here.

## Requirement
A single-operator web app that is both (1) a **kanban board** where each ticket is a task an
autonomous AI agent works on inside a registered repo, and (2) a **management system** for
agents, skills, and MCP servers. User picks the agent/model per task, monitors live progress,
reviews the agent's code, gives feedback, and an implementer↔reviewer agent loop converges
before a human opens the final PR.

## Success Criteria (observable "done")
- A user can register a project (local path or git URL), create a task, assign it to a
  configured agent, and watch the agent implement it live (streamed step log).
- The agent's code lands on a real repo branch (worktree per task); the user can review the
  diff, leave feedback, and the agent re-works the same branch until converged.
- A reviewer agent auto-loops findings back to the implementer; on APPROVE the user opens a PR.
- The user can manage Agents (persona/model/skills/MCP/tools), import Skills (markdown), and
  register MCP servers (stdio) — these attach to agents and take effect at run time.
- Multi-model: tasks run against any of OpenAI/Anthropic/Gemini/GLM using user-supplied keys.

## Acceptance Criteria (invariants the implementation must meet)
- **Isolation:** every agent `run_command` executes inside a per-task container that holds NO
  API keys and NO git credentials. Mutating git (commit/push/PR) is performed ONLY by the
  backend. (Prompt-injection cannot reach host or keys.)
- **Concurrency:** concurrent Doing tasks never corrupt each other — one worktree + one branch
  per task off the shared repo.
- **Bounded loops:** implementer↔reviewer ≤ 5 rounds; per-run ≤ ~50 tool calls + token cap;
  per-container ≤ 30-min wall-clock; user can Stop a running task (container killed).
- **No silent merge:** code never auto-merges/auto-PRs; PR creation is a human-initiated action.
- **Secrets:** provider API keys encrypted at rest; single shared git credential (system-level).
- **Realtime:** live agent step log reaches the browser via SSE and survives refresh.
- **Multi-provider uniformity:** one internal `{messages, tools}` shape translated per provider;
  model is selectable per task (overriding the agent default).

## Refined Requirement (grill-me findings — full decision log)

| # | Question | Decision |
|---|----------|----------|
| 1 | Task shape | Coding task (from an impl plan) **or** research task; real code output |
| 2 | Execution environment | Operates on the **real repo** (not a clone); affects the real repo |
| 3 | Concurrency model | **One git worktree + one branch per task**, off the shared repo |
| 4 | Projects | **Multiple projects**, user registers each |
| 5 | Repo provisioning | Accept **local path OR git URL** (clone when URL) |
| 6 | Git credentials | **Single shared SSH/deploy key** (system-level), all projects |
| 7 | Agent runtime | **Go-native thin agent loop** over provider SDKs (no external framework) |
| 8 | Providers | **All four**: OpenAI, Anthropic, Gemini, GLM (user supplies own API keys) |
| 9 | Key/auth model | **Single operator, no auth**; one encrypted key set |
| 10 | Execution model | **Async worker + SSE** step-log stream |
| 11 | Feedback loop | User reviews code → feedback → agent fixes ALL on same branch (sees prior thread) → converged ⇒ implementation done |
| 12 | Kanban columns | `Backlog → Doing → Review → Done` (+ Blocked/Cancelled) |
| 13 | Reviewer | A **second agent** reviews the implementation |
| 14 | Post-review flow | **Auto-fix loop** implementer↔reviewer until APPROVE; **human final merge gate** |
| 15 | Command isolation | **Container per task** for command execution |
| 16 | Persistence | **Postgres** for everything |
| 17 | Frontend | **Vite + React + TS SPA** against a **Go API** |
| 18 | Realtime transport | **SSE** |
| 19 | Merge mechanism | **PR-only** via `gh`/GitHub API (repo must have a GitHub remote) |
| 20 | Agent↔git split | **Backend owns all mutating git**; container is a credential-less edit+build sandbox driven via `docker exec` |
| 21 | Deployment | **docker-compose** (app + postgres; app spawns task containers as siblings via Docker socket) |
| 22 | (K8s reconciliation) | Moot — reverted from K8s to docker-compose |
| 23 | "Agent" entity | **Bundle**: persona/system-prompt + default model (overridable per task) + skills + MCP + allowed-tools; tasks assigned to an agent |
| 24 | Skills | **Markdown instruction modules** (Claude-Code-style), imported via **UI upload (file/zip)**, attached to agents, injected at run time (all attached skills for MVP) |
| 25 | MCP | **stdio-only** for MVP, attached to agents, **backend is the MCP client** (official Go SDK) brokering tools to the LLM; MCP servers run on the **backend host** |

## Assumptions (baked in unless contested)
- Light per-run token/cost tracking displayed on the ticket.
- Branch naming `agent/<task-id>-<slug>`; conventional commit messages.
- Research tasks produce a markdown artifact on the ticket (no code execution).
- Built-in tools (read_file/write_file/list_files/run_command) are toggleable per agent.
- Implementer & Reviewer ship as two built-in agent profiles.

## Phase 2 — Spec
- `docs/spec.md`, `docs/design.md`, `docs/tasks.md` written at repo root.
- Open change in `openspec/` deferred (no OpenSpec setup in this greenfield repo yet).

## Carried Forward (Not Verified / known caveats)
- No code exists yet — this is spec only. Implementation, review, PR are a separate engagement.
- loop-engineering's implement↔review loop can be run LATER, per Go service, once code exists.
- Anthropic has no official Go SDK → community SDK (`liushuangls/go-anthropic`) is a dependency risk to watch.
- RWX/PVC concerns avoided by choosing docker-compose over K8s.
