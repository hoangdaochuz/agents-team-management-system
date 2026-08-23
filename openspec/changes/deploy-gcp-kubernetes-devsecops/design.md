# Design: deploy-gcp-kubernetes-devsecops

## Context

Today the system runs only via `deploy/docker-compose.yml` (Postgres 16 + KRaft Kafka + the 11
services built from the shared `deploy/service.Dockerfile`, a distroless nonroot multi-stage
build parameterized by `SERVICE`/`PORT`). Relevant constraints discovered in exploration:

- **Config is env-only**, read directly via `os.Getenv` in each composition root
  (`backend/internal/platform/config/config.go` is stale and unused). Full env surface:
  `HTTP_ADDR`, `KAFKA_BROKERS`, `<SVC>_DB_DSN` per service, gateway `UPSTREAM_*`, runner
  `RUNNER_DRIVER|SANDBOX|SANDBOX_IMAGE|DOCKER_SOCKET|CLONE_ROOT|PR_BASE_URL|MAX_*`,
  `SETTINGS_MASTER_KEY|INTERNAL_TOKEN`, `AUTH_SEED_SUPERADMIN_*`.
- **The runner's Docker driver speaks raw HTTP over a unix socket** (`sandbox/docker.go`,
  `net.Dial("unix", ...)`, Docker Engine API v1.44, no TCP/TLS support). It creates sandbox
  containers with `Binds: [worktreePath:/workspace:rw]` and expects the sandbox image to exist
  locally on that daemon.
- **Worktrees are created host-side** via `exec.Command("git", ...)` against a pre-existing clone
  at `RUNNER_CLONE_ROOT` (no `git clone` in the backend; PR creation is a stub emitting
  `pr.opened`). File tools read/write the worktree path directly from the runner process — the
  runner and the Docker daemon **must share one filesystem**.
- **LLM provider keys are not env vars** — they live encrypted in the Settings service
  (`SETTINGS_MASTER_KEY`) and are fetched over an internal mTLS+token path. Only a small set of
  infra secrets must be provisioned.
- **Frontend**: `frontend/Dockerfile` references a missing `nginx.conf` (build is broken today);
  the gateway is API-only (no `go:embed`, no static serving).
- **CI**: `.github/workflows/ci.yml` runs Go vet/build/test-race/lint and frontend
  typecheck/build. No images, no deploy. Repo default branch is `master`; CI currently triggers on
  `main` — must be fixed as part of this change.
- **No IaC anywhere**; `openspec/specs/` is empty (all capabilities in this change are new).

User-locked decisions: Cloud SQL + **Managed Service for Apache Kafka**; **Argo CD** GitOps;
**full Terraform**; **dev + prod**; **real Docker sandbox in-cluster**; **nginx-in-GKE SPA**;
DevSecOps pipeline flow Semgrep → Gitleaks → Dependabot → test/build → Docker build → Trivy gate
→ Syft SBOM → Cosign → registry → Kyverno → Production → Falco.

## Goals / Non-Goals

**Goals:**
- One reproducible path from Git commit → scanned, signed image → running workload on GKE.
- The full system works on GKE *including real agent execution* (Docker sandbox), preserving the
  credential-less-sandbox invariant.
- Security gates that fail closed: nothing unsigned, unscanned, or misconfigured runs.
- Dev and prod as Kustomize overlays over one base, promoted by digest.

**Non-Goals:**
- Multi-region prod, DR/backup strategy beyond Cloud SQL defaults, cost optimization.
- Observability stack (Prometheus/Grafana/tracing) — only liveness/readiness/Argo health; a
  separate change can add metrics.
- Backend refactors. The only permitted backend touches: none planned (see D5 fallback), plus CI
  workflow files and `frontend/nginx.conf`.
- Changing the local docker-compose dev environment.

## Decisions

### D1 — Pipeline shape maps the user's flow onto GitHub Actions + GCP
Stages in one workflow (`.github/workflows/ci.yml` extended, plus a `deploy.yml`): PR stage runs
Semgrep (docker://semgrep/semgrep with `p/default` + Go rulesets), Gitleaks, go vet/build/`test
-race`/golangci-lint, frontend typecheck/build, `terraform fmt/validate`, `kustomize build` for
all overlays, Trivy config/IaC scan. Main-branch stage builds 13 images (11 services via
`deploy/service.Dockerfile` matrix, SPA, sandbox base from `backend/runner/Dockerfile`), tags
`sha-<commit>`, runs Trivy image scan (exit on CRITICAL), generates Syft SBOMs, Cosign keyless
signs image+SBOM (GitHub OIDC), pushes to Artifact Registry via **GitHub OIDC → GCP** (WIF, no
keys), then updates the dev overlay's digests in Git (or a separate manifests repo — see D6).
*Alternative rejected:* GitLab CI / Cloud Build — repo already lives on GitHub Actions.
*Assumption recorded:* the "GHCR" box in the user's flow diagram becomes **Artifact Registry**
(GCP-native, required for the Kyverno origin check); swapping registries later is a one-line
policy change.

### D2 — Terraform layout under `infra/`
```
infra/
  modules/           # vpc, gke, artifact-registry, cloudsql, managed-kafka, filestore, secrets, wif
  envs/dev/          # main.tf + terraform.tfvars + backend config (GCS state)
  envs/prod/
```
Shared modules, per-env state buckets. Key resources: GKE Autopilot is **rejected** — the sandbox
needs privileged DinD, so we use a **Standard Zonal (dev) / Regional (prod) cluster** with: a
default pool (Spot allowed in dev), and a `sandbox` pool with taint `aaks/sandbox=true:NoSchedule`
on a machine type sized for Docker builds (e.g. `e2-standard-4`). Cloud SQL private-IP Postgres
16, 10 logical DBs via the `postgresql` provider (mirroring `deploy/postgres/01-create-databases.sql`).
Managed Service for Apache Kafka with the per-domain topics from the contracts. Filestore
Basic HDD/SSD for the clone root. Secret Manager entries for infra secrets. WIF bindings for CI
OIDC + per-workload KSA→GSA mappings. *Alternative rejected:* Cloud SQL via `google_sql_database`
only for the instance DB + init SQL — fragile with migrations; per-DB resources are explicit.

### D3 — Runner sandbox topology (the hard part)
Single-replica runner Deployment, `replicas: 1` (worktree/file access assumes one node's view;
scaling is an open question OQ1). Pod spec:
- **DinD sidecar**: `docker:dind` image (pinned digest), `privileged: true`, `DOCKERD_ROOTLESS`
  off, storage on a dedicated `emptyDir` (or PD), socket at `/dind/docker.sock` shared via
  `emptyDir` volume; DinD pre-pulls the sandbox image at startup (initContainer `docker pull` of
  the pinned `aaks-runner` digest through that socket).
- **Runner container**: `RUNNER_SANDBOX=docker`, `RUNNER_DOCKER_SOCKET=/dind/docker.sock` (already
  configurable — no code change), `RUNNER_SANDBOX_IMAGE=<registry>/sandbox@sha256:...`,
  `RUNNER_CLONE_ROOT=/clone` where `/clone` is the Filestore PVC (ReadWriteMany).
- **Clone bootstrap**: a CronJob or one-shot Job that `git clone`s the managed repos into the
  Filestore clone root (the backend never clones). `git` must exist in the runner image —
  **distroless has no git**, and the runner execs `git worktree` host-side. Decision: switch the
  runner's image build (compose file only, service.Dockerfile gains a `GIT_INSTALLED` variant or
  the runner uses a second Dockerfile based on `alpine` + git + ca-certs, still nonroot). This is
  the one build-pipeline change required; it is packaging, not backend logic.
- Bind-mount path: sandbox.go binds `wt.Path` (host path as seen by the *runner*) into the
  container at `/workspace`. With DinD, the Docker daemon is a sibling container; the bind source
  path must be valid **inside the DinD container's mount namespace**. Therefore the Filestore PVC
  is mounted at the **same absolute path (`/clone`) in both the runner container and the DinD
  sidecar**, so `RUNNER_CLONE_ROOT=/clone` is identical in both namespaces. Verified against
  sandbox.go behavior: worktree paths derive from `RUNNER_CLONE_ROOT`, so this alignment makes
  the bind resolvable by the daemon.
- *Alternative rejected:* mounting the node's docker.sock — weaker isolation, node-level blast
  radius, and GKE nodes' containerd socket isn't a Docker socket at all. *Alternative rejected:*
  runner-on-VM — splits the delivery story and needs TCP/TLS Docker support the code lacks.

### D4 — SPA serving: fix `frontend/nginx.conf`
Create `frontend/nginx.conf`: listen `8080` as non-root (`nginx` uid), `gzip` on, SPA
`try_files ... /index.html`, `/api/` → `proxy_pass http://gateway.<ns>.svc:8080` with
`proxy_buffering off`, `proxy_read_timeout` long, `proxy_http_version 1.1` (SSE-safe). Deploy as
its own Deployment+Service; one GCE Ingress (or Gateway API) with a Google-managed cert routing
`/api`+`/stream` → gateway, `/` → SPA. Single origin, no CORS. *Alternative rejected:* go:embed —
requires backend change and couples releases; bucket+CDN — CORS complexity for SSE.

### D5 — Secrets: External Secrets Operator + Secret Manager
ESO installed via Argo CD; `SecretStore` per namespace authenticated via Workload Identity.
ExternalSecrets for: per-service Cloud SQL DSNs (composed from Secret Manager DB password +
Terraform-known host/db), `SETTINGS_MASTER_KEY`, `SETTINGS_INTERNAL_TOKEN`,
`AUTH_SEED_SUPERADMIN_*`. Seeded once via `gcloud secrets create` (documented manual bootstrap of
values; Terraform creates the secret containers with rotation metadata). Nothing secret in the
GitOps repo.

### D6 — GitOps repo layout
Manifests live in-tree at `k8s/` (same repo as code — this project's OpenSpec flow prefers one
repo; a split manifests repo is a later refactor):
```
k8s/
  base/<svc>/            # Deployment+Service+PDB+ConfigMap per workload, 12 workloads
  base/migrations-job/
  components/            # argocd app-of-apps, kyverno policies, eso, trivy-operator, falco
  overlays/dev/          # namespace, image digests (kustomize edit set image), env config
  overlays/prod/
```
CI promotes by committing digest updates to `overlays/dev`; prod promotion is a manual PR that
bumps `overlays/prod` digests (Argo CD auto-sync dev, auto-sync prod with self-heal but sync
windows optional). Argo CD bootstrapped by a one-time `kubectl apply` of the app-of-apps; after
that, everything flows through Git.

### D7 — Kyverno over OPA/Gatekeeper; Falco with rules tuned for the sandbox
Kyverno policies: verify-images (Artifact Registry origin + Cosign keyless attestation via the
 Fulcio/Rekor chain), restrict `latest`, require non-root + resource limits, block privileged
 outside the `runner-sandbox` namespace, block hostPath/host-socket except the declared runner
 volumes. Falco runs on nodes (DaemonSet); default ruleset minus noisy container-egg-spawn rules
 triggered by legitimate DinD behavior in the sandbox namespace, plus a custom rule alerting on
 shell spawns in the 11 service containers. Trivy Operator scans workloads; findings ≥ HIGH
 surface as Kubernetes vulnerability reports (alerting wiring deferred — Non-Goal observability).

### D8 — Migrations
Each service's migrations live in its repo path
(`infrastructure/repository/migrations/`). Migration Job per service in `base/migrations-job/`
running `migrate` (or the service binary's migrate subcommand if one exists — none found; so the
Job uses the service image with a `migrate` entrypoint shim or a `golang-migrate` image reading
the repo's migration files via initContainer checkout). Ordering: Jobs run before the service
Deployment's new pods become ready (Argo sync waves + `PreSync` hooks). *Simplest honest option:*
Argo CD `PreSync` hook Job per service running `golang-migrate` with migrations mounted from a
ConfigMap generated from the repo at CI time.

## Risks / Trade-offs

- [DinD socket + shared-filesystem coupling is fragile] → integration test in dev env that runs a
  full agent task (task → Doing → sandbox container → run_command → steps streamed); the existing
  `sandbox/secret_leak_test.go` runs against dev too.
- [NFS (Filestore) latency for git operations and builds] → acceptable for agent workloads; if
  builds are IO-bound, move to a per-node PD with the runner pinned via nodeSelector (documented
  escape hatch).
- [Runner single replica = availability gap] → acceptable now (agent runs are queueable); scaling
  needs per-runner clone partitions — OQ1.
- [Kyverno verify-images with keyless cosign adds startup latency and a Fulcio/Rekor dependency]
  → cache policy results; if it proves flaky, fall back to a fixed cosign key stored in Secret
  Manager (policy swap is contained).
- [Managed Kafka IAM/SASL auth vs the services' plaintext client config] → services currently
  assume PLAINTEXT brokers; Managed Kafka supports SASL/PLAIN or IAM. Decision: enable
  SASL/PLAIN with per-env credentials delivered via External Secrets; requires adding SASL config
  to the Kafka platform layer (`internal/platform/kafka`) — the **one backend change** this change
  must make (env-driven `KAFKA_SASL_*`), flagged in tasks as a scoped code edit.
- [Two environments cost money continuously] → dev sized minimal (Spot nodes, single-zone,
  Filestore Basic HDD, Cloud SQL db-g1-small; Kafka smallest capacity).
- [In-tree manifests repo couples app + deploy commits] → promotion PRs are small and reviewable;
  split-repo refactor deferred.

## Migration Plan

1. Merge infra Terraform → `terraform apply` dev → prod (no workload impact; nothing exists yet).
2. Add nginx.conf, runner Dockerfile variant, Kafka SASL support; CI publishes first images.
3. Bootstrap Argo CD + app-of-apps in dev → dev runs end-to-end including a real agent task.
4. Verify DevSecOps controls in dev (policy rejections behave, Falco/Trivy report).
5. Prod apply + promotion PR. Rollback = revert overlay commit (Git) or `argocd app rollback`.

## Open Questions

- OQ1: Runner horizontal scaling (partitioning clone roots / DinD per replica) — defer; single
  replica for now, documented.
- OQ2: GCE Ingress vs Gateway API for HTTPS entry — decide at implementation; Gateway API is the
  GKE-forward choice, Ingress is simpler. Either satisfies the spec.
- OQ3: Alert routing for Falco/Trivy findings (email/Slack/PubSub) — deferred with observability;
  spec only requires queryable visibility.
