# Proposal: deploy-gcp-kubernetes-devsecops

## Why

The system runs today only via `deploy/docker-compose.yml` on a single developer machine. There is
no codified infrastructure, no image publishing, no deployment pipeline, and no security gating
between a merged commit and what runs in production. We need the full system (5 consolidated Go
services — gateway, identity, workspace, agent, executor — React SPA, Cloud SQL, managed Kafka,
and the Docker sandbox agent-execution path) running on GCP under a DevSecOps discipline — every
artifact scanned, signed, and policy-checked before it runs, and every environment reproducible
from Git. (Revised after `consolidate-microservices` landed: the original 11-service topology
became 5 services over 4 logical databases and 9 Kafka topics.)

## What Changes

- **Terraform IaC for the full GCP landing zone**: GKE cluster (with a dedicated privileged node
  pool for the agent sandbox), Artifact Registry, Cloud SQL (PostgreSQL) with 4 logical databases
  (`identity_db`, `workspace_db`, `agent_db`, `runner_db`), Managed Service for Apache Kafka,
  Filestore RWX volume for the managed git clone, Secret Manager, Workload Identity bindings,
  VPC/firewall, and a dev + prod environment layout.
- **Kubernetes deployment manifests (Kustomize)** for all 5 backend services (gateway, identity,
  workspace, agent, executor), the SPA (fixed `frontend/nginx.conf` serving `dist/` and
  reverse-proxying `/api` to the Gateway), the executor sandbox (Docker-in-Docker sidecar +
  Filestore-mounted clone root), ingress/HTTPS, health probes, resource requests/limits, and
  PodDisruptionBudgets.
- **CI/CD pipeline in GitHub Actions** extending the existing `ci.yml`: Semgrep SAST, Gitleaks
  secret scanning, Dependabot SCA, `go test`/`go vet`/lint, `npm` typecheck/build, Docker build of
  all service images, Trivy CVE gate (fail on critical), Syft SBOM generation, Cosign keyless
  signing, publish to Artifact Registry, and Argo CD GitOps promotion dev → prod.
- **DevSecOps runtime controls**: Kyverno cluster policies (only signed images from our Artifact
  Registry may run; no privileged pods outside the sandbox pool; enforce resource limits,
  non-root, no host-path escapes), Trivy Operator in-cluster workload scanning, Falco runtime
  detection, External Secrets Operator syncing from GCP Secret Manager, NetworkPolicies, and Pod
  Security Standards.
- **Secrets moved out of config**: all runtime secrets (DB credentials, `AGENT_MASTER_KEY`,
  `INTERNAL_TOKEN`, `AGENT_INTERNAL_TOKEN`, seed superadmin, LLM provider keys flow) sourced from
  GCP Secret Manager via External Secrets Operator — never in Git or images.

## Capabilities

### New Capabilities

- `gcp-infrastructure`: Terraform modules and environment layout that provision the GCP landing
  zone — GKE, Artifact Registry, Cloud SQL, Managed Kafka, Filestore, Secret Manager, IAM/Workload
  Identity, VPC — reproducibly for dev and prod.
- `kubernetes-deployment`: Kubernetes (Kustomize) manifests that run all 5 consolidated services,
  the SPA, and the agent-execution sandbox on GKE with production-grade reliability (probes,
  resources, replicas, PDBs, ingress) and the executor's Docker sandbox (DinD + shared clone
  volume).
- `cicd-pipeline`: GitHub Actions pipeline that builds, tests, scans, signs, and publishes
  immutable images for every commit, and promotes them through GitOps.
- `devsecops-controls`: The security gates and runtime controls — Semgrep, Gitleaks, Dependabot,
  Trivy, Syft, Cosign, Kyverno admission policy, Trivy Operator, Falco, secret management, cluster
  hardening — enforced across the pipeline and the cluster.
- `gitops-delivery`: Argo CD-based declarative delivery with dev and prod environments,
  image promotion by digest, drift detection, and rollback.

### Modified Capabilities

(none — `openspec/specs/` is empty; all five capabilities above are new)

## Impact

- **New top-level directory** `infra/` (Terraform), `k8s/` (Kustomize manifests), and additions to
  `.github/workflows/` (pipeline jobs; existing `build-test`/`frontend` jobs keep running).
- **`deploy/`** stays as the local docker-compose dev environment — unchanged.
- **`frontend/Dockerfile`** gains the currently-missing `nginx.conf` (port 8080, `/api` proxy,
  SSE-friendly buffering off); no frontend source changes.
- **`backend/`** requires no code changes for the core deploy; the executor sandbox runs with
  `EXECUTOR_SANDBOX=docker` against a DinD sidecar unix socket and a Filestore-backed
  `EXECUTOR_CLONE_ROOT` (code change needed only if DinD socket path/behavior diverges — tracked in
  design).
- **External systems**: GCP project (billing, APIs), GitHub repo settings (OIDC trust to GCP,
  Dependabot, secret scanning), Argo CD installed in-cluster.
- **Cost/ops**: Cloud SQL HA, Managed Kafka, Filestore, and GKE node pools bill continuously;
  dev environment sized small, prod sized for availability.
