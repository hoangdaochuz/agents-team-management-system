## Purpose

Defines how the system's GCP infrastructure is provisioned declaratively with Terraform: the GKE
cluster, managed data services (Cloud SQL, Managed Kafka), registry, storage, secrets, and IAM,
reproducibly for both dev and prod environments.

## ADDED Requirements

### Requirement: Infrastructure is fully codified in Terraform
Every GCP resource the system depends on (GKE cluster and node pools, Artifact Registry, Cloud SQL
instance and databases, Managed Service for Apache Kafka cluster and topics, Filestore instance,
Secret Manager secrets, service accounts and IAM bindings, VPC/subnets/firewall rules) SHALL be
defined in Terraform modules under version control. No environment-critical resource SHALL exist
only as a manual `gcloud`/console action.

#### Scenario: Fresh environment bootstrap
- **WHEN** a new engineer with GCP credentials runs `terraform init` and `terraform apply` against
  a new environment's tfvars
- **THEN** Terraform reports a complete plan and applies it with zero pre-existing manual
  resources required beyond the GCP project itself and enabled APIs

#### Scenario: Drift is detectable
- **WHEN** a resource is modified manually in the GCP console
- **THEN** a subsequent `terraform plan` shows the drift as a pending change

### Requirement: Two environments — dev and prod
Terraform SHALL support a dev and a prod environment from the same modules, differing only in
variable values (instance sizes, replica counts, HA settings), with separate GCP resource names
and no shared state.

#### Scenario: Environments are isolated
- **WHEN** prod infrastructure is destroyed and recreated
- **THEN** dev infrastructure and data are unaffected

#### Scenario: Prod data services are highly available
- **WHEN** the prod environment is applied
- **THEN** Cloud SQL runs HA and the GKE cluster spans at least three zones

### Requirement: GKE cluster with a dedicated sandbox node pool
The GKE cluster SHALL have a default node pool for the 5 services and the SPA, and a separate
tainted, privileged node pool for the agent-execution sandbox (Docker-in-Docker), so that sandbox
workloads are the only workloads eligible to run privileged.

#### Scenario: Regular services never schedule onto the sandbox pool
- **WHEN** a standard service pod is created
- **THEN** it schedules only onto the default pool, because the sandbox pool's taint repels it

#### Scenario: Executor pod schedules onto the sandbox pool
- **WHEN** the executor pod (with the matching toleration) is created
- **THEN** it schedules onto the sandbox node pool

### Requirement: Managed data plane — Cloud SQL and Managed Kafka
PostgreSQL SHALL be provided by Cloud SQL with private IP in the VPC, hosting the 4 logical
service databases (`identity_db`, `workspace_db`, `agent_db`, `runner_db`). Kafka SHALL be
provided by Managed Service for Apache Kafka in the VPC, with
the topics the services consume (including `__consumer_offsets` and the per-domain event topics)
provisioned. No database or Kafka broker SHALL run as a Kubernetes workload.

#### Scenario: Services reach the database privately
- **WHEN** a service pod connects using its DSN
- **THEN** the connection stays on private IP inside the VPC and no database port is exposed to
  the public internet

#### Scenario: Kafka topics exist before services start
- **WHEN** the environment is freshly applied and services deploy
- **THEN** the Kafka topics the consumers subscribe to already exist with the expected partition
  counts, so consumers do not depend on auto-creation

### Requirement: Registry and images
A GCP Artifact Registry Docker repository SHALL exist per environment (or one shared with
per-environment image naming), and every image deployed to the cluster SHALL come from it.

#### Scenario: Only registry images are deployable
- **WHEN** a deployment manifest references an image from outside the project's Artifact Registry
- **THEN** admission policy rejects it (enforced by the `devsecops-controls` capability)

### Requirement: Secrets live in Secret Manager
All runtime secrets (Cloud SQL credentials per service, `AGENT_MASTER_KEY`, `INTERNAL_TOKEN`,
`AGENT_INTERNAL_TOKEN`, seed superadmin credentials) SHALL be stored in GCP Secret Manager and
never in Git, Terraform variable files committed to the repo, or container images. Terraform SHALL
create secret *containers* (empty or randomly-generated values where safe) without hardcoding
secret values in source.

#### Scenario: No plaintext secret in the repo
- **WHEN** secret-scanning runs over the repository
- **THEN** no GCP secret value or production credential is found

### Requirement: Workload Identity, not keys
Every Kubernetes service account that needs GCP APIs (Argo CD image updater, External Secrets,
workload scanners) SHALL authenticate via GKE Workload Identity bindings. No GCP service account
key JSON SHALL be stored as a Kubernetes secret or committed to the repo.

#### Scenario: CI authenticates to GCP without a key
- **WHEN** the GitHub Actions pipeline pushes an image to Artifact Registry
- **THEN** it authenticates via GitHub OIDC federation to GCP, with no long-lived key anywhere

### Requirement: Shared clone volume for agent execution
A Filestore (NFS) volume SHALL be provisioned per environment to hold the managed git clone and
per-task worktrees, mounted read-write by the executor pod and bind-mountable into the sandbox
containers started by the executor's Docker daemon.

#### Scenario: Executor and sandbox share the same filesystem view
- **WHEN** the executor writes a file into a task worktree path and a sandbox container reads it at
  `/workspace`
- **THEN** both see the same file content
