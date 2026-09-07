## Purpose

Defines how the 5 consolidated backend services (gateway, identity, workspace, agent, executor),
the SPA frontend, and the agent-execution sandbox run on GKE with production-grade reliability:
declarative manifests, health probes, sizing, ingress, and the Docker sandbox topology for real
agent execution.

## ADDED Requirements

### Requirement: All services deploy declaratively to GKE
The 5 backend services (gateway, identity, workspace, agent, executor) and the SPA SHALL each
have a Kubernetes Deployment and Service defined in Kustomize manifests, with a base overlay
shared by dev and prod and per-environment overlays supplying images, config, and sizing.

#### Scenario: Base manifests render for both environments
- **WHEN** `kustomize build` runs against the dev and prod overlays
- **THEN** both render a complete, valid set of manifests for all 6 workloads with no manual
  patching required

#### Scenario: Service topology matches the compose contract
- **WHEN** the manifests are applied
- **THEN** each service receives its documented env surface (per-service DSN, `KAFKA_BROKERS`,
  gateway upstream URLs, executor sandbox settings) with values resolved per environment, and the
  gateway proxies `/api/<domain>/...` to the same upstream services it proxies in
  docker-compose

### Requirement: Every workload is production-grade
Every Deployment SHALL declare: image referenced by digest (immutable), liveness and readiness
probes on `/healthz`, CPU/memory requests and limits, ≥2 replicas for all services except the
executor, and a PodDisruptionBudget for multi-replica services. Deployments SHALL use rolling
updates and respect graceful shutdown.

#### Scenario: Pod failure is not an outage
- **WHEN** one replica of a multi-replica service is killed
- **THEN** the service remains available via its remaining replicas and the replacement pod passes
  readiness before receiving traffic

#### Scenario: Node drain does not drop all replicas
- **WHEN** a node is drained for upgrade
- **THEN** the PodDisruptionBudget prevents eviction of the last available replica of each service

### Requirement: The SPA is served by nginx behind the same ingress as the API
The frontend SHALL run as an nginx container serving the built `dist/` bundle and reverse-proxying
`/api` to the gateway service. The missing `frontend/nginx.conf` SHALL be created (listen 8080,
non-root, proxy buffering disabled for SSE). A single HTTPS ingress (managed certificate) SHALL
route `/api` and `/stream` to the gateway path and all other paths to the SPA.

#### Scenario: SPA and API on one origin
- **WHEN** a browser loads the application's public URL
- **THEN** the SPA is served from the same origin as the API, with no CORS preflight needed

#### Scenario: SSE streams through the proxy
- **WHEN** the SPA opens a task event stream (`/api/tasks/:id/stream`)
- **THEN** steps arrive incrementally through nginx and the ingress without buffering-induced delay

### Requirement: Agent-execution sandbox runs in-cluster
The executor SHALL run as a single-replica Deployment on the dedicated sandbox node pool with:
`EXECUTOR_SANDBOX=docker`, a Docker-in-Docker sidecar exposing its unix socket via a shared
`emptyDir` volume mounted at the path `EXECUTOR_DOCKER_SOCKET` expects, the sandbox base image
pre-pulled/available to that daemon, and `EXECUTOR_CLONE_ROOT` pointing at the Filestore PVC.
The sandbox containers the executor creates SHALL hold no API keys and no git credentials (the
credential-less invariant from `design.md` §3.4 is preserved).

#### Scenario: A Doing task gets a real sandbox
- **WHEN** a task moves to Doing and the executor starts an agent run
- **THEN** the executor creates a sandbox container from the sandbox image with the task worktree
  bind-mounted at `/workspace`, and executes build/test/edit commands inside it

#### Scenario: Sandbox containers are credential-less
- **WHEN** the secret-leak test runs against the in-cluster sandbox path
- **THEN** no provider API key, DB credential, or git credential is discoverable inside a sandbox
  container

#### Scenario: Executor pod is the only privileged workload
- **WHEN** any pod other than the executor (and its DinD sidecar) requests privileged security
  context or a host socket mount
- **THEN** admission policy rejects it (enforced by `devsecops-controls`)

### Requirement: Database migrations run as deploy steps
Schema migrations for the 4 service databases SHALL run as an ordered pre-deploy step (Kubernetes
Job or init sequence) before the new service version receives traffic, using the same image digest
as the service it migrates for.

#### Scenario: Migration precedes rollout
- **WHEN** a new service version is deployed that includes a schema migration
- **THEN** the migration job completes successfully before the new pods are marked ready

### Requirement: Config is injected at runtime
All environment-specific configuration (DSNs with private Cloud SQL IPs, Kafka bootstrap
endpoints, upstream URLs, seed values) SHALL come from ConfigMaps and External Secrets per
environment. The container image SHALL be identical across dev and prod.

#### Scenario: Same image, different environments
- **WHEN** comparing the dev and prod manifests for a service
- **THEN** the image digest is promotable unchanged and only ConfigMap/Secret references differ
