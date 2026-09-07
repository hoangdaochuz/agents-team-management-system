## Purpose

Defines how validated images reach dev and then prod: Argo CD reconciling declarative manifests
from Git, digest-based promotion, drift detection, and rollback — the CD half of the DevSecOps
flow where Git is the single source of truth for what runs in each environment.

## ADDED Requirements

### Requirement: Argo CD declaratively manages both environments
Argo CD SHALL run in the cluster and manage the dev and prod Applications from a declarative
app-of-apps (or ApplicationSet) bootstrap. The desired state of every environment SHALL be a
Git-tracked Kustomize overlay; no environment SHALL be modified by imperative `kubectl apply`
outside an emergency, and such emergency changes SHALL be visible as drift.

#### Scenario: Git commit deploys to dev
- **WHEN** CI merges to the default branch and updates the dev overlay's image digests
- **THEN** Argo CD syncs the dev Application to the new digests without manual commands

#### Scenario: Manual cluster edit is detected
- **WHEN** someone edits a deployment's image directly with kubectl
- **THEN** Argo CD reports the application as OutOfSync and (with auto-sync) reverts it

### Requirement: Promotion dev → prod is an explicit Git action
An image SHALL enter dev automatically after CI passes, and enter prod only when a promotion
commit/PR updates the prod overlay to a digest that has passed its time/configured soak in dev.
Promotion SHALL be a reviewable Git change (PR), preserving the auditable trail of who promoted
what, when.

#### Scenario: Promotion PR carries provenance
- **WHEN** a prod promotion PR is opened
- **THEN** it references the exact commit SHA/digest, links to the CI run that built, scanned, and
  signed it, and requires review approval before merge

#### Scenario: Unsoaked image is not promotable
- **WHEN** the promotion tooling is asked to promote a digest that has not deployed to dev
- **THEN** promotion refuses

### Requirement: Rollback is a Git revert
Rollback in either environment SHALL be achievable by reverting the overlay commit (Argo CD
re-syncs the previous known-good digests), in addition to Argo CD's built-in history rollback.

#### Scenario: Bad release is rolled back
- **WHEN** a prod release misbehaves and its overlay commit is reverted
- **THEN** the cluster converges back to the previous image digests and the change is recorded in
  Git history

### Requirement: Secrets stay out of the GitOps repo
The Git-tracked overlays SHALL reference secrets only through ExternalSecret resource names; sync
secrets are created in-cluster by External Secrets Operator, never committed.

#### Scenario: GitOps repo scan is clean
- **WHEN** secret scanning runs over the manifests repository
- **THEN** no secret material is found

### Requirement: Deploy health is observable
Each Argo CD Application SHALL have sync and health status for all workloads visible in the Argo
CD UI/CLI; a failed rollout (crashlooping or never-ready pods) SHALL surface as degraded
application health rather than a silently stale environment.

#### Scenario: Failed rollout is visible
- **WHEN** a newly synced version crashloops in prod
- **THEN** the Application reports degraded health with the failing workload named
