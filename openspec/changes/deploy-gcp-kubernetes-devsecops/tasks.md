# Tasks: deploy-gcp-kubernetes-devsecops

Ordered by dependency: IaC first, then packaging fixes, CI, manifests/GitOps, security controls,
environments, verification. Reference `specs/` for the behavioral bar and `design.md` (D1–D8) for
the how.

## 1. Terraform foundation (design D2)

- [x] 1.1 Create `infra/modules/vpc` (network, subnet with secondary ranges for GKE
  Pods/Services, Cloud NAT) and `infra/modules/artifact-registry` (Docker repo)
- [x] 1.2 Create `infra/modules/gke`: Standard cluster (private nodes, Workload Identity,
  Security Posture on) with default pool + tainted `sandbox` node pool
  (`aaks/sandbox=true:NoSchedule`)
- [x] 1.3 Create `infra/modules/cloudsql`: private-IP Postgres 16 instance + 10 logical databases
  (mirror `deploy/postgres/01-create-databases.sql`) + secret in Secret Manager for the DB
  password
- [x] 1.4 Create `infra/modules/managed-kafka`: Managed Service for Apache Kafka cluster +
  per-domain topics (from `backend/internal/contracts/events`) with SASL/PLAIN credentials in
  Secret Manager
- [x] 1.5 Create `infra/modules/filestore` (clone-root NFS volume) and `infra/modules/secrets`
  (SETTINGS_MASTER_KEY, SETTINGS_INTERNAL_TOKEN, AUTH_SEED_SUPERADMIN_* secret containers)
- [x] 1.6 Create `infra/modules/wif`: GitHub OIDC provider/pool for CI, per-workload
  KSA→GSA Workload Identity bindings (Argo CD, ESO, trivy-operator)
- [x] 1.7 Create `infra/envs/dev` and `infra/envs/prod` (main.tf, tfvars sized per design,
  GCS state backends, remote state locking); document `terraform init/apply` runbook in
  `infra/README.md`

## 2. Packaging fixes (design D3/D4)

- [x] 2.1 Create `frontend/nginx.conf` (port 8080 non-root, SPA fallback, `/api` proxy to
  gateway with `proxy_buffering off` + HTTP/1.1 for SSE); fix `frontend/Dockerfile` port/user;
  verify `docker build` succeeds and the image serves
- [x] 2.2 Add runner image variant with `git` present (alpine-based, nonroot, ca-certs) used for
  the runner Deployment; keep `deploy/service.Dockerfile` for the other 10 services unchanged
- [x] 2.3 Add SASL/PLAIN support to `backend/internal/platform/kafka` driven by env
  (`KAFKA_SASL_MECHANISM/USER/PASSWORD_*`), plaintext path untouched when unset; unit test with
  unset vars proving current behavior is unchanged
- [x] 2.4 Pin all base images (golang, distroless, nginx, alpine, docker:dind) by digest in the
  Dockerfiles

## 3. CI/CD pipeline — PR gates (design D1)

- [x] 3.1 Fix CI trigger branch (`main` → `master`) on `.github/workflows/ci.yml`; add job-level
  concurrency and path-aware triggers
- [x] 3.2 Add Semgrep SAST job (backend + frontend, fail on error severity)
- [x] 3.3 Add Gitleaks secret-scanning job; enable GitHub secret scanning + push protection +
  Dependabot (`.github/dependabot.yml` for gomod + npm + actions)
- [x] 3.4 Add IaC/manifest validation job: `terraform fmt -check`/`validate` on `infra/`,
  `kustomize build` on all overlays, Trivy config scan on `k8s/`

## 4. CI/CD pipeline — build, scan, sign, publish (design D1)

- [x] 4.1 Create `.github/workflows/deploy.yml`: on push to master after ci gates — build 11
  service images (matrix over SERVICE/PORT via `deploy/service.Dockerfile`), SPA image, sandbox
  image; tag `sha-<commit>`
- [x] 4.2 Authenticate to GCP via GitHub OIDC → WIF (no stored keys); push to Artifact Registry
- [x] 4.3 Add Trivy image-scan gate (fail on CRITICAL) before images are promoted; attach report
  to the run
- [x] 4.4 Add Syft SBOM generation and Cosign keyless signing of image digest + SBOM; record
  provenance; verify with `cosign verify` in a smoke step
- [x] 4.5 Add auto-promotion step: `kustomize edit set image` on `k8s/overlays/dev` with the new
  digests + commit

## 5. Kubernetes manifests — base (design D6/D8)

- [x] 5.1 Scaffold `k8s/base` + `k8s/overlays/{dev,prod}` Kustomize layout with namespace/
  commonLabels/namePrefix conventions
- [x] 5.2 Create Deployment+Service+PDB+ConfigMap for the 11 backend services: digest image,
  `/healthz` probes, requests/limits, ≥2 replicas (runner 1), rolling update, graceful shutdown
  (terminationGracePeriodSeconds), SA with `automountServiceAccountToken: false`
- [x] 5.3 Create the runner Deployment: sandbox-pool toleration/nodeSelector, DinD sidecar
  (privileged, digest-pinned, socket via shared emptyDir, pre-pull initContainer), Filestore PVC
  mounted at `/clone` in BOTH runner and DinD containers, env per design D3
  (`RUNNER_SANDBOX=docker`, `RUNNER_DOCKER_SOCKET=/dind/docker.sock`, `RUNNER_CLONE_ROOT=/clone`)
- [x] 5.4 Create clone-bootstrap CronJob/Job that seeds the managed repos into the Filestore
  clone root
- [x] 5.5 Create SPA Deployment+Service (nginx image) and HTTPS ingress routing `/api`→gateway,
  `/`→SPA with managed certificate (Ingress or Gateway API per OQ2)
- [x] 5.6 Create migration PreSync Job resources per service (golang-migrate with migrations from
  CI-generated ConfigMap), wired as Argo CD PreSync hooks
- [x] 5.7 Create ExternalSecret resources for all runtime secrets (per-service DSNs composed from
  Secret Manager values, SETTINGS_*, AUTH_SEED_*) — references only, no values

## 6. Cluster services + GitOps (design D6)

- [x] 6.1 Add Argo CD install + app-of-apps bootstrap under `k8s/components/argocd`; document
  one-time bootstrap command
- [x] 6.2 Define Argo Applications for dev (auto-sync + self-heal) and prod (auto-sync, promotion
  via PR); annotate base resources with sync waves (migrations before services)

## 7. DevSecOps controls (design D7)

- [x] 7.1 Install Kyverno via Argo CD; add policies: verify-images (Artifact Registry origin +
  Cosign signature), deny `latest`, require non-root + requests/limits, block privileged/hostPath
  outside the runner-sandbox path — with narrowly-scoped exceptions for DinD
- [x] 7.2 Define default-deny + allow-list NetworkPolicies per design (service→SQL/Kafka,
  gateway→upstreams, SPA→gateway, control-plane tools)
- [x] 7.3 Install Trivy Operator via Argo CD; verify vulnerability reports appear per namespace
- [x] 7.4 Install Falco via Argo CD with tuned rules (suppress expected DinD churn in the sandbox
  namespace; custom alert on shell spawns in the 11 service containers); verify alert pipeline is
  queryable
- [x] 7.5 Apply Pod Security Standards labels (restricted where possible, baseline explicitly for
  system + sandbox namespaces)

## 8. Environments up + end-to-end verification

- [ ] 8.1 `terraform apply` dev; bootstrap Argo CD; confirm all 12 workloads healthy on GKE dev
- [ ] 8.2 Verify dev data plane: services connect to Cloud SQL privately, Kafka consumers attach
  to managed topics, ExternalSecrets sync, no secret material in Git (run gitleaks over k8s/)
- [ ] 8.3 Verify real agent execution in dev: bootstrap clone, move a task to Doing, confirm
  sandbox container spawns from the signed image with the worktree bind-mounted at /workspace,
  steps stream to the SPA over SSE; run the secret-leak invariant check against the in-cluster
  sandbox
- [ ] 8.4 Verify admission gates behave: attempt an unsigned foreign image and a `latest`-tagged
  pod — both rejected with policy messages
- [ ] 8.5 `terraform apply` prod; promote digests via PR; verify prod healthy + rollback drill
  (revert promotion commit, confirm convergence)
- [x] 8.6 Update `CLAUDE.md`/`AGENTS.md` with the deploy layout (infra/, k8s/, pipelines) and the
  runbook links
