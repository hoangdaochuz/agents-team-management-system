## Purpose

Defines the security controls enforced at admission time and at runtime on the cluster — Kyverno
policy, workload scanning, runtime detection, secret delivery, and cluster hardening — so that
only scanned, signed, correctly-configured workloads can run, and suspicious behavior is detected
in production.

## ADDED Requirements

### Requirement: Admission policy admits only trusted images
A Kyverno (or equivalent) admission policy SHALL reject any pod whose image is not from the
project's Artifact Registry AND does not carry a valid Cosign signature from the project's CI
identity. System namespaces and the sandbox DinD image are handled via explicitly-scoped
exceptions.

#### Scenario: Unsigned foreign image is rejected
- **WHEN** someone applies a deployment using `nginx:latest` from Docker Hub
- **THEN** admission denies the pod with a policy-violation message naming the failed check

#### Scenario: Signed registry image is admitted
- **WHEN** a deployment references a digest from the project's Artifact Registry signed by CI
- **THEN** the pod is admitted

### Requirement: Cluster-wide workload hardening policy
Cluster policy SHALL require, for every namespace in scope: containers run as non-root with a
read-only root filesystem where feasible, `latest` tags are forbidden, resource requests and
limits are set, and no pod may mount the host Docker socket or hostPath /proc / sys except the
runner sandbox path. Privileged containers SHALL only be allowed in the sandbox node pool's
workload. Pod Security Standards SHALL be enforced (baseline/restricted) with the sandbox
namespace exempted and separately guarded.

#### Scenario: Root container is rejected
- **WHEN** a pod spec omits runAsNonRoot or requests uid 0 outside the sandbox namespace
- **THEN** admission rejects it

#### Scenario: latest tag is rejected
- **WHEN** a manifest references a mutable tag
- **THEN** admission rejects it

### Requirement: In-cluster vulnerability scanning of running workloads
Trivy Operator (or equivalent) SHALL run in the cluster and continuously scan deployed images and
exposed secrets in workloads, reporting results per namespace, with a defined severity threshold
that raises a visible finding/alert.

#### Scenario: Newly disclosed CVE on a running image
- **WHEN** a CVE at or above the threshold is published for an image already running in prod
- **THEN** the scanner reports it against that workload within its next scan cycle, visible in the
  cluster's vulnerability report

### Requirement: Runtime threat detection
Falco (or equivalent) SHALL run on cluster nodes and raise alerts on runtime behavior rules —
container spawning a shell in unexpected namespaces, writing to binary paths, credential-file
access — with alerts routed to a visible destination (at minimum, queryable logs).

#### Scenario: Suspicious exec in a service pod
- **WHEN** a shell is spawned inside one of the 5 service containers
- **THEN** Falco raises a runtime alert identifying the pod and container

### Requirement: Secrets delivered from Secret Manager at runtime
External Secrets Operator SHALL sync each needed secret from GCP Secret Manager into the
namespace-scoped Kubernetes Secrets the workloads consume, on a refresh interval. No real secret
value SHALL appear in Git, Helm values committed to the repo, or image layers.

#### Scenario: Secret rotation reaches pods
- **WHEN** a secret's value is rotated in Secret Manager
- **THEN** External Secrets refreshes the Kubernetes Secret within its refresh interval, and pods
  that mount/env-from it pick up the new value on their next restart or reload

#### Scenario: Manifests contain no secret material
- **WHEN** the Kustomize/GitOps repository is audited
- **THEN** only secret *references* (ExternalSecret names) exist; every value resolves at runtime

### Requirement: Network segmentation
Each environment SHALL apply default-deny NetworkPolicies between namespaces where practical, with
explicit allow rules for: service → Cloud SQL (private IP), service → Kafka, gateway → upstream
services, SPA → gateway, runner → sandbox container network traffic, and control-plane scanning
tools → workload namespaces.

#### Scenario: Cross-namespace probe is blocked
- **WHEN** a pod in one environment's namespace tries to connect to a service in the other
  environment's namespace
- **THEN** the connection is denied by policy

### Requirement: Least-privilege workload identity
Each workload (or workload group) that requires GCP access SHALL have its own Kubernetes service
account bound via Workload Identity to a GCP service account scoped to the minimum roles — e.g.
External Secrets to Secret Manager accessor only — and pods run with `automountServiceAccountToken`
disabled where no GCP/K8s API access is needed.

#### Scenario: Compromised pod cannot exfiltrate broadly
- **WHEN** an attacker gains code execution in a service pod with no GCP API need
- **THEN** the pod has no usable GCP credentials and cannot read Secret Manager or storage
