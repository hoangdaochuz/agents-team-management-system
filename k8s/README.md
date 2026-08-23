# Kubernetes Manifests for AI Agent Kanban System

This directory contains declarative Kubernetes/Kustomize manifests for deploying the AI Agent Kanban System (AAKS) to Google Kubernetes Engine (GKE). The manifests implement the full DevSecOps pipeline including Argo CD GitOps, Kyverno admission policies, External Secrets Operator, Trivy Operator, and Falco runtime security.

## Directory Layout

```
k8s/
  base/                    # Base manifests shared across environments
    gateway/              # Gateway BFF deployment + service + PDB
    project/              # Project service
    task/                 # Task service
    agent/                # Agent service
    catalog/              # Catalog service
    settings/             # Settings service
    runner/               # Runner service with DinD sidecar (design D3)
    auth/                 # Auth service
    orgs/                 # Orgs service
    resources/            # Resources service
    admin/                # Admin service
    web/                  # Frontend SPA nginx deployment
    migrations/           # Argo CD PreSync migration jobs
    kustomization.yaml   # Base composition

  components/             # Cluster-wide components (namespaced)
    argocd/               # Argo CD install + app-of-apps
    kyverno/              # Admission policies (verify-images, restrict-privileged, etc.)
    network-policies/     # Default-deny + allow rules
    external-secrets/     # ESO install + SecretStore + ExternalSecrets
    trivy-operator/       # Vulnerability scanning
    falco/                # Runtime threat detection
    pss/                  # Pod Security Standards namespace labels

  overlays/
    dev/                  # Dev environment overlay
      namespace.yaml
      filestore-storage-class.yaml
      ingress.yaml        # GCE Ingress + managed certificate
      env-configmap.yaml  # Kafka brokers, runner config
      kustomization.yaml  # Image digest placeholders (sha-PLACEHOLDER)
    prod/                 # Prod environment overlay
      # Same structure as dev, with larger sizing
```

## Design Decisions Implemented

### Runner Sandbox (Design D3)

The runner workload runs with a Docker-in-Docker sidecar:

- **DinD sidecar**: `docker:dind` image (pinned digest), privileged mode, storage on dedicated `emptyDir`
- **Shared socket**: `/dind/docker.sock` mounted in both DinD and runner containers
- **Filestore PVC**: Mounted at `/clone` in both DinD and runner for worktree access
- **InitContainer**: Pre-pulls the sandbox image at startup
- **Toleration**: `aaks/sandbox=true:NoSchedule` taint for dedicated sandbox node pool
- **Environment**: `RUNNER_SANDBOX=docker`, `RUNNER_DOCKER_SOCKET=/dind/docker.sock`, `RUNNER_CLONE_ROOT=/clone`

This preserves the credential-less-sandbox invariant—the runner holds all secrets and git credentials; sandbox containers see only the worktree.

### Migrations (Design D8)

Each service has an Argo CD PreSync hook Job running `golang-migrate/migrate`:

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
- Default-deny ingress/egress in namespace `aaks`
- Allow gateway → upstream services
- Allow web → gateway
- Allow all services → Cloud SQL (private IP, patched per env)
- Allow all services → Kafka
- Allow ESO → GCP control plane
- Allow DNS

**Falco** rules:
- Suppress expected DinD/sandbox churn for pods labeled `aaks/sandbox`
- Alert on shell spawns in the 11 service containers

### Secrets (Design D5)

External Secrets Operator syncs secrets from GCP Secret Manager:

- **Per-service DSNs**: `gateway-db-dsn`, `project-db-dsn`, etc.
- **Settings secrets**: `settings-master-key`, `settings-internal-token`
- **Auth seeds**: `auth-seed-superadmin-email`, `auth-seed-superadmin-password`
- **SecretStore per env**: Workload Identity authentication

No secret values appear in Git.

## Promotion Process

### 1. CI Builds and Pushes Images

On merge to `master`, CI:
1. Builds 13 images (11 services + SPA + sandbox base)
2. Tags with `sha-<commit>`
3. Runs Trivy scan (exits on CRITICAL)
4. Generates SBOMs
5. Signs with Cosign keyless (GitHub OIDC → GCP)
6. Pushes to Artifact Registry

### 2. CI Updates Dev Overlay

CI updates `overlays/dev/kustomization.yaml`:

```bash
kustomize edit set image gateway=us-docker.pkg.dev/PROJECT/aaks/gateway:sha-<ACTUAL_DIGEST>
# ... for all 12 workloads
```

CI commits and pushes to `master`. Argo CD auto-syncs dev.

### 3. Prod Promotion (Manual PR)

To promote to prod:

1. Create a branch from the dev digest commit
2. Update `overlays/prod/kustomization.yaml` with the same digests
3. Open a PR referencing the CI run and the soak time in dev
4. After review and approval, merge
5. Argo CD syncs prod (auto-sync enabled, selfHeal disabled)

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
# 1. Apply the Argo CD installation and app-of-apps
kustomize build k8s/components/argocd | kubectl apply -f -

# 2. Wait for Argo CD to be ready
kubectl wait --for=condition=available --timeout=300s \
  deployment/argocd-server -n argocd

# 3. Port-forward to Argo CD UI
kubectl port-forward svc/argocd-server -n argocd 8080:443

# 4. Login with admin credentials
argocd login localhost:8080 \
  --username admin \
  --password $(kubectl get secret argocd-initial-admin-secret \
    -n argocd -o jsonpath='{.data.password}' | base64 -d)

# 5. Sync the dev app (auto-sync will handle future updates)
argocd app sync aaks-dev
```

The app-of-apps will:
1. Create the `aaks` namespace
2. Apply all base manifests with dev overlay
3. Apply all components (Kyverno, ESO, Trivy, Falco)
4. Run migration jobs (PreSync hooks)
5. Start all service pods

### Environment-Specific Values

Update the following per environment in `overlays/<env>/env-configmap.yaml`:

- `kafka-brokers`: Kafka bootstrap servers
- Cloud SQL private IP ranges in network policies
- ExternalSecret `SecretStore` references
- Ingress domain names

## Validation

Validate manifests before applying:

```bash
# Validate dev overlay
kustomize build k8s/overlays/dev | kubectl apply --dry-run=server -f -

# Validate prod overlay
kustomize build k8s/overlays/prod | kubectl apply --dry-run=server -f -
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
# For each service
kubectl create configmap project-migrations \
  --from-file=../../migrations-artifacts/project/ \
  --dry-run=client -o yaml
```

## Troubleshooting

**Migration job fails**: Check job logs for connection errors to Cloud SQL. Verify ExternalSecret `SecretStore` is correctly configured.

**Pod stuck in ImagePullBackOff**: Kyverno policy rejected the image. Check `kubectl describe pod` for policy violation details. Verify image is signed and from Artifact Registry.

**Falco alerts on shell spawns in sandbox**: Expected. Use label `aaks/sandbox: "true"` to suppress these.

**Runner cannot create sandbox containers**: Verify DinD sidecar is healthy, socket is shared at `/dind/docker.sock`, and Filestore PVC is mounted.

## References

- Design: `openspec/changes/deploy-gcp-kubernetes-devsecops/design.md`
- Kubernetes spec: `openspec/changes/deploy-gcp-kubernetes-devsecops/specs/kubernetes-deployment/spec.md`
- DevSecOps spec: `openspec/changes/deploy-gcp-kubernetes-devsecops/specs/devsecops-controls/spec.md`
- GitOps spec: `openspec/changes/deploy-gcp-kubernetes-devsecops/specs/gitops-delivery/spec.md`
