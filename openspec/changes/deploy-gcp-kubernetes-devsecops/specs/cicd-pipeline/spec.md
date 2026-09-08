## Purpose

Defines the GitHub Actions pipeline that takes a commit from pull request to a signed, published,
deployable artifact: code security checks, build and test, image build, CVE gate, SBOM, signing,
and registry publication — the CI half of the DevSecOps flow.

## ADDED Requirements

### Requirement: Pull-request security checks gate every PR
Every pull request SHALL run, and fail the PR on findings above the configured threshold:
Semgrep SAST on backend and frontend source, Gitleaks secret scanning on the full diff and
history, and Dependabot alerts/SCA on Go and npm dependencies. Repository secret scanning and
Dependabot SHALL be enabled in repo settings.

#### Scenario: Vulnerable dependency blocks merge
- **WHEN** a PR introduces a Go or npm dependency with a known critical vulnerability
- **THEN** the pipeline's SCA check fails the PR with the offending dependency identified

#### Scenario: Leaked secret blocks merge
- **WHEN** a PR adds a line matching known secret patterns (API keys, credentials)
- **THEN** Gitleaks fails the check and the PR cannot merge until the secret is removed

#### Scenario: Semgrep finding blocks merge
- **WHEN** Semgrep reports an error-severity finding in changed code
- **THEN** the SAST job fails the PR

### Requirement: Existing quality gates keep running
The pipeline SHALL retain the current CI gates: `go mod tidy` check, `go vet`, `go build`,
`go test -race`, golangci-lint, frontend typecheck, and frontend build. Image build and deploy
jobs SHALL only run after these pass on the default branch.

#### Scenario: Test failure stops the pipeline
- **WHEN** any Go test fails
- **THEN** no Docker image is built or published for that commit

### Requirement: Build once, promote the immutable artifact
For every commit to the default branch, the pipeline SHALL build each container image exactly once
— the 5 service images (4 via the shared service Dockerfile, the executor via the git-carrying
variant), the SPA image, and the sandbox base
image — tagged with the Git commit SHA, and publish them to Artifact Registry authenticated via
GitHub OIDC (no static GCP keys). Environments SHALL receive the same digest; promotion changes
the manifest reference, never rebuilds.

#### Scenario: Image tag maps to commit
- **WHEN** inspecting a deployed image in any environment
- **THEN** its SHA tag (and digest) resolves to exactly one Git commit, and dev and prod running
  the same version run byte-identical images

#### Scenario: Pipeline authenticates without stored credentials
- **WHEN** the publish job runs
- **THEN** it exchanges the GitHub OIDC token for short-lived GCP credentials and no service
  account key exists in repository secrets

### Requirement: Vulnerability gate before publication
Before any image is pushed (or immediately after push, before it becomes deployable), the pipeline
SHALL scan every image with Trivy and fail if any CRITICAL (or settable threshold) CVE is present
in the final image. Base images SHALL be pinned by digest.

#### Scenario: Critical CVE blocks the image
- **WHEN** Trivy finds a critical CVE in a built image
- **THEN** the pipeline fails, the image is not promoted, and the report is attached to the run

#### Scenario: Clean image passes
- **WHEN** an image has no CVEs at or above the threshold
- **THEN** the pipeline proceeds to SBOM and signing

### Requirement: SBOM and signing for every image
The pipeline SHALL generate an SBOM with Syft and sign both the image digest and the SBOM with
Cosign keyless signing for every published image. SBOMs and signatures SHALL be stored/queryable
(attached to the registry image or as workflow artifacts).

#### Scenario: Provenance is verifiable
- **WHEN** verifying a deployed image with `cosign verify`
- **THEN** a valid keyless signature exists whose certificate identity matches the repo's GitHub
  Actions workflow, and the SBOM for that digest is retrievable

### Requirement: IaC is validated in CI
Terraform and Kustomize changes SHALL be gate-checked in CI: `terraform fmt`/`validate` (and
`plan` against dev where credentials allow), `kustomize build` for all overlays, and Trivy
IaC/config scanning of the k8s manifests.

#### Scenario: Broken manifest blocks merge
- **WHEN** a PR breaks `kustomize build` for any overlay or Terraform validation
- **THEN** the IaC job fails the PR
