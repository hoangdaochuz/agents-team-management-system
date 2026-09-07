# Kubernetes Manifests for AI Agent Kanban System

This directory contains declarative Kubernetes/Kustomize manifests for deploying the AI Agent Kanban System (AAKS) to Google Kubernetes Engine (GKE). The manifests implement the full DevSecOps pipeline including Argo CD GitOps, Kyverno admission policies, External Secrets Operator, Trivy Operator, and Falco runtime security.

## Directory Layout

```
k8s/
  base/                    # Workloads in `aaks` (PSS restricted)
    gateway/              # Gateway BFF deployment + service + PDB
    identity/             # Identity service (auth + orgs + admin merged)
    workspace/            # Workspace service (project + task + catalog + resources merged)
    agent/                # Agent service (agent + settings merged)
    web/                  # Frontend SPA nginx deployment
    migrations/           # Argo CD PreSync migration jobs (one per database)
    kustomization.yaml   # Base composition (no executor — see sandbox/)

  sandbox/                 # Privileged plane in `aaks-sandbox` (PSS privileged,
                           # Kyverno-guarded — PSS cannot exempt a single pod)
    base/
      executor/           # Executor + DinD sidecar + Filestore /clone PVC (design D3)
      clone-bootstrap/    # CronJob seeding the managed repos into /clone (design D3)
      externalsecrets.yaml # Sandbox secrets (same ClusterSecretStore)
      networkpolicies.yaml # Default-deny + allows for aaks-sandbox
      namespace.yaml      # aaks-sandbox + PSS labels
    overlays/
      dev/                # executor/clone digests, sandbox env-values, small PVC
      prod/               # larger DinD/executor sizing, 1Ti PVC

  components/             # Cluster-wide services + policy (per-cluster)
    argocd/apps/          # AppProject + root-dev/root-prod + platform/operator,
                          # config, and workload/sandbox Applications
    kyverno/              # Admission policies (verify-images, restrict-privileged, etc.)
    network-policies/     # Default-deny + allow rules for `aaks`
    external-secrets/     # ClusterSecretStore + ExternalSecrets for `aaks`
    trivy-operator/       # values.yaml (consumed by the Argo Helm app)
    falco/                # values.yaml with tuned customRules (Argo Helm app)
    pss/                  # Pod Security Standards labels (system namespaces;
                          # workload namespaces are owned by their overlays)

  overlays/
    dev/                  # Dev workloads overlay
      namespace.yaml
      filestore-storage-class.yaml
      ingress.yaml        # GCE Ingress + managed certificate
      env-configmap.yaml  # Kafka brokers, runner config
      kustomization.yaml  # Image digest placeholders (sha-PLACEHOLDER)
    prod/                 # Prod workloads overlay (same shape, larger sizing)
```

## Design Decisions Implemented

### Executor Sandbox (Design D3)

The executor workload (the runner, renamed in `consolidate-microservices`) runs in the
**dedicated `aaks-sandbox` namespace** (`k8s/sandbox/`, overlaid per env) — NOT next to the
services. Reason: Pod Security Standards admit no per-pod exceptions, and the privileged
DinD sidecar is inadmissible under the `restricted` profile the `aaks` namespace enforces.
The sandbox namespace runs PSS `privileged` (warn/audit `baseline`) and is guarded instead by
the label-scoped Kyverno policies (`aaks/sandbox=true`), its own default-deny NetworkPolicies,
and the tainted sandbox node pool (`aaks/sandbox=true:NoSchedule` taint; nodes carry the
slash-free `aaks-sandbox=true` label for the executor's nodeSelector — GCE labels forbid `/`).

- **DinD sidecar**: `docker:dind` image (pinned digest), privileged mode, storage on dedicated `emptyDir`
- **Shared socket**: `/dind/docker.sock` mounted in both DinD and executor containers
- **Filestore PVC**: Mounted at `/clone` in both DinD and executor for worktree access
- **InitContainer**: Pre-pulls the sandbox image at startup
- **Toleration**: `aaks/sandbox=true:NoSchedule` taint for dedicated sandbox node pool
- **Environment**: `EXECUTOR_SANDBOX=docker`, `EXECUTOR_DOCKER_SOCKET=/dind/docker.sock`, `EXECUTOR_CLONE_ROOT=/clone`
- **Cross-namespace upstreams**: the executor reaches agent/workspace (and the gateway reaches
  the executor) via cluster FQDNs (`*.aaks.svc.cluster.local` / `*.aaks-sandbox.svc.cluster.local`),
  allowed by the cross-namespace NetworkPolicy rules on both sides.

This preserves the credential-less-sandbox invariant—the executor holds all secrets and git credentials; sandbox containers see only the worktree.

### Clone bootstrap (Design D3, task 5.4)

The backend never `git clone`s — it only creates worktrees inside a pre-existing clone.
`sandbox/base/clone-bootstrap/` is an hourly CronJob (CI-built `clone` image: git on pinned
alpine) that clones every repo in the sandbox `env-values` `clone-repos` key on first run and
`git fetch`es afterwards. Bootstrap a fresh environment immediately with:

```bash
kubectl -n aaks-sandbox create job --from=cronjob/clone-bootstrap clone-bootstrap-manual
```

Private repos need a `git-credentials` Secret (key `token`) in `aaks-sandbox` — the CronJob's
reference is optional and injects the token into https remotes at runtime (never baked in).

### Migrations (Design D8)

Each service has an Argo CD PreSync hook Job running the CI-built `migrate` image
(`deploy/migrate.Dockerfile`: upstream golang-migrate, digest-pinned, repushed to our
Artifact Registry so Kyverno verify-images admits the Jobs):

- **Hook annotation**: `argocd.argoproj.io/hook: PreSync`
- **Delete policy**: `BeforeHookCreation`
- **Migration source**: ConfigMap populated by CI from `backend/services/<svc>/internal/infrastructure/repository/migrations`

Jobs run before the service Deployment's new pods become ready.

### Security Controls

**Kyverno policies** enforce:
- **verify-images**: Artifact Registry origin + Cosign keyless attestation (Fulcio/Rekor)
- **disallow-latest**: Blocks mutable tags
- **require-non-root**: All containers run as non-root (except sandbox)
- **require-resource-limits**: CPU/memory requests and limits required
- **restrict-privileged**: Blocks privileged pods except those labeled `aaks/sandbox: "true"`
- **restrict-hostpath**: Blocks hostPath mounts except for runner volumes

**Network policies** enforce:
- Default-deny ingress/egress in `aaks` AND `aaks-sandbox`
- Same-namespace backend ingress (services → each other; web → gateway) plus GCE
  LB/health-check ranges → gateway/web:8080
- Gateway (aaks) → executor:8086 and executor → agent:8081/workspace:8083 across namespaces
- Migration Jobs → Cloud SQL (they carry their own `app:migrations` selector)
- Services → Cloud SQL / Kafka private IPs (CIDRs patched per env)
- Sandbox plane → 443/22 outside RFC1918 (AR image pulls, git remotes) + DNS
- Allow DNS (kube-dns) everywhere

**Falco** rules (`components/falco/values.yaml`, shipped via the Argo Helm app):
- Except `executor-*` pods from the two noisiest default rules (Terminal shell,
  Container Drift) — expected DinD/sandbox churn
- Custom alert on shell spawns in the 5 service containers

### Secrets (Design D5)

A single cluster-scoped `ClusterSecretStore` (`aaks-secretstore`, Workload Identity) serves
both namespaces; `ExternalSecret`s live next to their consumers (`components/external-secrets`
for `aaks`, `sandbox/base/externalsecrets.yaml` for `aaks-sandbox`):

- **Per-service DSNs**: `identity-db-dsn`, `workspace-db-dsn`, `agent-db-dsn`, `executor-db-dsn` (the gateway has no DB)
- **Internal tokens**: `internal-token` (shared by all services), `agent-internal-token` (agent service internal endpoints, consumed by the executor)
- **Agent secrets**: `agent-master-key` (provider-key encryption)
- **Auth seeds**: `auth-seed-superadmin-email`, `auth-seed-superadmin-password` (consumed by identity)
- **ClusterSecretStore**: one per cluster, Workload Identity authentication (no keys)

No secret values appear in Git.

## Promotion Process

### 1. CI Builds and Pushes Images

On merge to `master`, CI:

1. Builds 9 images (5 services + SPA + sandbox base + clone + migrate)
2. Tags with `sha-<commit>`
3. Runs Trivy scan (exits on CRITICAL)
4. Generates SBOMs
5. Signs with Cosign keyless (GitHub OIDC → GCP)
6. Pushes to Artifact Registry

### 2. CI Updates Dev Overlays

CI resolves each `sha-<commit>` tag to its digest and updates both dev overlays:

```bash
kustomize edit set image gateway=us-docker.pkg.dev/PROJECT/aaks/gateway@sha256:<DIGEST>
# ... in k8s/overlays/dev for gateway/identity/workspace/agent/web/migrate,
# ... in k8s/sandbox/overlays/dev for executor/clone,
# plus the sandbox base image digest into the sandbox env-values ConfigMap
# (executor-sandbox-image is config, not a workload image).
```

CI commits and pushes to `master`. Argo CD auto-syncs dev (workloads + sandbox apps).

### 3. Prod Promotion (Manual PR)

To promote to prod: copy the dev digests (workloads + sandbox overlays AND the sandbox
env-values digest) into `k8s/overlays/prod` + `k8s/sandbox/overlays/prod` in one PR
referencing the CI run and the soak time in dev. After review and approval, merge;
Argo CD syncs prod (auto-sync enabled, selfHeal disabled — a human owns prod).

### Rollback

Rollback is a Git revert:

```bash
git revert <commit-that-updated-overlays>
# Argo CD re-syncs to previous digests
```

Or use Argo CD's built-in rollback via CLI/UI.

## Bootstrap Instructions

### Prerequisites

- GKE cluster created via Terraform (`infra/envs/dev`)
- GCP Workload Identity configured
- Argo CD CLI installed
- kubectl configured for the cluster

### One-Time Bootstrap

```bash
# 1. Install Argo CD itself from the pinned upstream manifest (v2.12.4):
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/v2.12.4/manifests/install.yaml

# 2. Register the app-of-apps root for THIS cluster's env (dev cluster gets
# root-dev, prod cluster gets root-prod — each root excludes the other env):
kubectl apply -f k8s/components/argocd/apps/appproject.yaml
kubectl apply -f k8s/components/argocd/apps/root-dev.yaml   # or root-prod.yaml

# 3. Wait for Argo CD to be ready
kubectl wait --for=condition=available --timeout=300s \
  deployment/argocd-server -n argocd

# 4. Port-forward to Argo CD UI
kubectl port-forward svc/argocd-server -n argocd 8080:443

# 5. Login with admin credentials
argocd login localhost:8080 \
  --username admin \
  --password $(kubectl get secret argocd-initial-admin-secret \
    -n argocd -o jsonpath='{.data.password}' | base64 -d)

# 6. Sync the dev plane (auto-sync handles future updates)
argocd app sync aaks-root-dev
```

The app-of-apps will (ordered by sync-wave: operators → configs → workloads):
1. Install the pinned operators (Kyverno, External Secrets, Trivy Operator, Falco) from Helm
2. Sync ClusterSecretStore/ExternalSecrets, Kyverno policies, NetworkPolicies, PSS labels
3. Create the `aaks` + `aaks-sandbox` namespaces
4. Run migration jobs (PreSync hooks)
5. Start all service + sandbox pods

### Environment-Specific Values

Update the following per environment in `overlays/<env>/env-configmap.yaml` and
`sandbox/overlays/<env>/env-configmap.yaml`:

- `kafka-brokers`: Kafka bootstrap servers (both env-values ConfigMaps)
- Cloud SQL / Kafka private IP ranges in the NetworkPolicies
- `aaks-ENV-*` remote key names in the ExternalSecrets (`ENV` = dev/prod)
- `clone-repos`: managed repo URLs for the clone-bootstrap CronJob
- Ingress domain names

## Validation

Validate manifests before applying (all four overlays plus the components —
this is what CI's iac-validate job runs):

```bash
# Validate all overlays
for o in overlays/dev overlays/prod sandbox/overlays/dev sandbox/overlays/prod; do
  kustomize build k8s/$o | kubectl apply --dry-run=server -f -
done

# Validate components
for c in argocd argocd/apps kyverno network-policies external-secrets pss; do
  kustomize build k8s/components/$c > /dev/null && echo "$c OK"
done
```

## Image Registry Placeholder

During development, images are referenced as `sha-PLACEHOLDER`. CI replaces these with actual digests. To test locally:

```bash
kustomize build k8s/overlays/dev | \
  sed 's/sha-PLACEHOLDER/latest' | \
  kubectl apply -f -
```

## Migration Files

Migration ConfigMaps are populated by CI at deploy time. The placeholder files exist so `kustomize build` succeeds. CI runs:

```bash
# For each service with a database
kubectl create configmap identity-migrations \
  --from-file=../../migrations-artifacts/identity/ \
  --dry-run=client -o yaml
```

## Troubleshooting

**Migration job fails**: Check job logs for connection errors to Cloud SQL. Verify the
ClusterSecretStore + ExternalSecrets synced (ESO logs) and the migration ConfigMap is populated.

**Pod stuck in ImagePullBackOff**: Kyverno policy rejected the image. Check `kubectl describe pod` for policy violation details. Verify image is signed and from Artifact Registry.

**Falco alerts on shell spawns in the sandbox plane**: Expected for DinD churn — the
`tuned customRules` in `components/falco/values.yaml` except pods named `executor-*` from the
two noisiest default rules. Alerts naming other pods are real findings.

**Executor cannot create sandbox containers**: Verify DinD sidecar is healthy, socket is shared at `/dind/docker.sock`, Filestore PVC is mounted at `/clone` in both containers, and the clone-bootstrap CronJob has seeded the repo (`kubectl -n aaks-sandbox logs job/<clone-job>`).

## References

- Design: `openspec/changes/deploy-gcp-kubernetes-devsecops/design.md`
- Kubernetes spec: `openspec/changes/deploy-gcp-kubernetes-devsecops/specs/kubernetes-deployment/spec.md`
- DevSecOps spec: `openspec/changes/deploy-gcp-kubernetes-devsecops/specs/devsecops-controls/spec.md`
- GitOps spec: `openspec/changes/deploy-gcp-kubernetes-devsecops/specs/gitops-delivery/spec.md`
