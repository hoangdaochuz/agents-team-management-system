---
pr: 1
pr_url: https://github.com/hoangdaochuz/agents-team-management-system/pull/1
branch: feat/event-driven-backend
base: master
date: 2026-08-12
openspec_change: event-driven-microservices-backend
review: none
---

# Impact: event-driven microservices backend (11 services, Kafka saga, multi-tenant, runner sandbox)

## Changed Surface
- **Files / packages:** every `backend/services/*` gained real HTTP APIs + pgx stores; new packages `backend/internal/httputil/scope.go`, `backend/services/runner/internal/{driver,mcp,sandbox,store}`, `backend/services/auth|orgs|admin|resources|settings|runner/internal/store`, `deploy/e2e/e2e.sh`, `deploy/service.Dockerfile`.
- **Public API changes:** gateway now exposes auth/orgs/admin endpoints; `POST /api/auth/signup` is the first endpoint of the whole system (there was no public API before this branch — the frontend SPA was built against a declared-but-unimplemented contract).
- **Schema:** 10 logical Postgres DBs, each with migrations (migrations/0001_init.sql everywhere; `0002_workspace_scope.sql` on task/agent/project/catalog; `0003_*` on task/agent/catalog). All applied at boot by `internal/db` migrator. No data existed before — first deployment of every table.
- **Proto:** none.
- **Config / env:** `deploy/docker-compose.yml` defines 11 service containers + postgres + kafka KRaft + `kafka-init` one-shot + e2e. Gateway fails fast at boot if any `UPSTREAM_*` env var is unset. `RUNNER_SANDBOX=docker|local`, `RUNNER_DRIVER=simulated|llm`, `AAKS_SANDBOX_TEST_DOCKER=1` (test-only), `AAKS_KAFKA_TEST_BROKERS` (test-only).

## Correctness Risks & Blast Radius
### High
- **Credential-less sandbox invariant** — the runner container must never receive provider keys/git tokens (Settings is the sole decryptor; sandbox containers get only the bind-mounted worktree). *Where:* `services/runner/internal/sandbox/docker.go` + `settings` internal key endpoint. *When:* if anyone adds `Env:` to the container spec or the settings internal API leaks keys outside the mTLS channel. *Trace:* task 7.4 `secret_leak_test.go` asserts env/filesystem/logs. Security-critical: regression here = provider API keys exposed to unvetted task code.
- **Scoping enforcement** — every store query must carry `X-Workspace-ID(s)` via `internal/httputil/scope.go`; services fail closed on empty workspace sets. *Where:* task/agent/project/catalog stores, `whereScopedAt`/`clauses()` patterns. *When:* any new store method added without the scope clause silently becomes cross-tenant. *Trace:* `0002_workspace_scope` migrations + E2E isolation task 14.2. Multi-tenant data-leak risk.
- **Kafka partition-per-task ordering** — lifecycle/step topics partitioned by task_id; saga (task service) consumes facts and emits commands. *Where:* `internal/contracts/events.go`, `internal/kafka/kafka.go`, task saga coordinator. *When:* a topic added with a different partitioner, or a consumer missing the idempotency hook, breaks per-task ordering / double-apply. At-least-once + idempotency is load-bearing.

### Medium
- **`__consumer_offsets` bootstrap dependency** — KRaft auto-create loops on the offsets topic; `kafka-init` one-shot pre-creates it and all services `depends_on` its completion. *Where:* `deploy/docker-compose.yml`. *When:* if `kafka-init` is removed or the broker address changes, every consumer group stalls with "coordinator is not available" (the exact outage this PR fixed). Local-deploy only, but a CI/deploy surprise if compose is copied elsewhere.
- **Simulated reviewer round semantics** — approval at `RoundNo >= 1` yields exactly 2 cycles/4 runs; changing `RoundNo` in `driver.go` silently alters the review-loop E2E contract (task 9.3). Cosmetic for real runs (LLM driver decides independently) but E2E-coupled.
- **Auth/gateway session chain** — signup approval → session cookie → `/internal/identity` → memberships cache (60s). *Where:* `gateway/internal/httpapi/routes.go` (cookie copy is manual), auth internal endpoints. *When:* a stale 60s memberships cache or a missed cookie copy breaks login/authorization silently. Noted: cookie propagation was a real bug fixed in this PR (`copyHeaders`).
- **Docker 29+ exec/start framing** — daemon may answer 200+chunked instead of a hijacked raw stream; `execStart` now parses via `http.ReadResponse`. *Where:* `sandbox/docker.go`. *When:* docker version-dependent; older hijack-style daemons also handled, but unverified against one.

### Low
- **E2E leftover rows** — the suite wipes auth/orgs users only; task/project/agent/runner DBs accumulate rows in deleted old workspaces. Harmless (scoped lists filter) but grows on repeated runs.
- **Gateway startup log** — upstreams line tries to JSON-marshal a func value; benign, logs an error at boot only.

## Traceability
- **Spec / task brief:** openspec change `event-driven-microservices-backend` → `proposal.md`, `design.md`, `tasks.md` (70/70 checked; 4.3 deferred by design).
- **Design:** `docs/design.md` (superseded by the OpenSpec change).
- **Review:** none (no go-backend-reviewer run on this branch).
- **Related PRs / issues:** none (first PR on this branch).
- **Key commits:** `46dfa35` feat: implement event-driven microservices backend (11 services, Kafka saga, auth/multi-tenant, runner sandbox + MCP).

## Not Verified
- E2E suite (37 assertions) and the docker sandbox test were executed only against the local docker compose stack with `simulated` runner driver — never in CI (CI has no docker daemon).
- The `llm` runner driver is untested end-to-end (no real provider keys exercised; only unit-level tool wiring).
- Docker sandbox verified against Docker 29.5.3 only; exec/start hijack-style framing on older daemons is code-handled but untested.
- No load/race testing of the Kafka consumers under `-race` beyond `go test ./...` (CI adds `-race` on the GitHub side).
- Kafka consumer offset/idempotency behavior after a broker restart mid-saga is untested.
