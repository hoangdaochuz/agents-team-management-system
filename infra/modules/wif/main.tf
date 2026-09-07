# WIF Module - Workload Identity Federation for GitHub Actions and Kubernetes workloads

locals {
  # Per-environment pool/provider: Workload Identity Pool IDs are global to
  # the project, so dev and prod sharing a project must not collide.
  pool_id     = "github-pool-${var.environment}"
  provider_id = "github-provider-${var.environment}"
}

# GitHub Actions Workload Identity Pool
resource "google_iam_workload_identity_pool" "github_pool" {
  project                   = var.project_id
  workload_identity_pool_id = local.pool_id
  display_name              = "GitHub Actions Workload Identity Pool"
  description               = "OIDC pool for GitHub Actions CI/CD"
  disabled                  = false

  lifecycle {
    ignore_changes = [disabled]
  }
}

# GitHub Actions Workload Identity Provider
resource "google_iam_workload_identity_pool_provider" "github_provider" {
  project                            = google_iam_workload_identity_pool.github_pool.project
  workload_identity_pool_id          = google_iam_workload_identity_pool.github_pool.workload_identity_pool_id
  workload_identity_pool_provider_id = local.provider_id
  display_name                       = "GitHub Actions OIDC Provider"
  description                        = "OIDC provider for GitHub Actions"

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }

  attribute_mapping = {
    "google.subject"   = "assertion.sub"
    "repository_owner" = "assertion.repository_owner"
    "repository_name"  = "assertion.repository"
    "ref"              = "assertion.ref"
    "sha"              = "assertion.sha"
    "workflow"         = "assertion.workflow"
    "environment"      = "assertion.environment"
    "actor"            = "assertion.actor"
  }

  # NOTE: GitHub sends `assertion.repository` as `owner/repo` — comparing
  # against the bare repo name never matches and CI OIDC is always denied.
  attribute_condition = "assertion.repository_owner == '${var.github_owner}' && assertion.repository == '${var.github_owner}/${var.github_repo}'"
}

# Service account for CI (GitHub Actions)
resource "google_service_account" "ci_sa" {
  project      = var.project_id
  account_id   = "ci-${var.environment}-sa"
  display_name = "CI Service Account for ${var.environment}"
  description  = "Service account for GitHub Actions CI/CD via WIF"
}

# CI SA: Artifact Registry Writer (for pushing images)
resource "google_project_iam_member" "ci_artifact_registry_writer" {
  project = var.project_id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${google_service_account.ci_sa.email}"
}

# NOTE: no `cloudsql.client` / `iam.serviceAccountUser` grants — CI pushes
# images only; migrations run in-cluster via Workload Identity. Least privilege.

# WIF binding: CI SA -> GitHub Actions
resource "google_service_account_iam_member" "ci_wif_binding" {
  service_account_id = google_service_account.ci_sa.id
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github_pool.name}/${google_iam_workload_identity_pool_provider.github_provider.name}:attribute.repository_owner/${var.github_owner}/attribute.repository/${var.github_repo}"
}

# Kubernetes Service Accounts and their GCP counterparts for GitOps workloads

# Argo CD Service Account (for image updates)
resource "google_service_account" "argocd_sa" {
  project      = var.project_id
  account_id   = "argocd-${var.environment}-sa"
  display_name = "Argo CD Service Account for ${var.environment}"
  description  = "Service account for Argo CD Workload Identity"
}

# Argo CD SA: Artifact Registry Reader
resource "google_project_iam_member" "argocd_artifact_registry_reader" {
  project = var.project_id
  role    = "roles/artifactregistry.reader"
  member  = "serviceAccount:${google_service_account.argocd_sa.email}"
}

# Argo CD SA: GKE Access (Kubernetes Engine Viewer for basic sync)
resource "google_project_iam_member" "argocd_gke_viewer" {
  project = var.project_id
  role    = "roles/container.viewer"
  member  = "serviceAccount:${google_service_account.argocd_sa.email}"
}

# WIF binding for Argo CD. Form is
# `serviceAccount:PROJECT.svc.id.goog[NAMESPACE/KSA]` — the bare
# `serviceAccount:ns/sa` form never matches and the binding stays dead.
# KSA `argocd-server` in namespace `argocd` (Argo CD install manifest).
resource "google_service_account_iam_member" "argocd_wif_binding" {
  service_account_id = google_service_account.argocd_sa.id
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[argocd/argocd-server]"
}

# External Secrets Operator Service Account
resource "google_service_account" "external_secrets_sa" {
  project      = var.project_id
  account_id   = "external-secrets-${var.environment}-sa"
  display_name = "External Secrets Operator Service Account for ${var.environment}"
  description  = "Service account for External Secrets Operator"
}

# External Secrets SA: Secret Manager Accessor (only what ESO needs — no
# `secretmanager.viewer`; listing project metadata is not required to sync).
resource "google_project_iam_member" "external_secrets_secret_accessor" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.external_secrets_sa.email}"
}

# WIF binding for External Secrets. KSA `external-secrets` in namespace
# `external-secrets` (ESO Helm release) — one binding, correct form.
resource "google_service_account_iam_member" "external_secrets_wif_binding" {
  service_account_id = google_service_account.external_secrets_sa.id
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[external-secrets/external-secrets]"
}

# Trivy Operator Service Account
resource "google_service_account" "trivy_operator_sa" {
  project      = var.project_id
  account_id   = "trivy-operator-${var.environment}-sa"
  display_name = "Trivy Operator Service Account for ${var.environment}"
  description  = "Service account for Trivy Operator image scanning"
}

# Trivy Operator SA: Artifact Registry Reader
resource "google_project_iam_member" "trivy_artifact_registry_reader" {
  project = var.project_id
  role    = "roles/artifactregistry.reader"
  member  = "serviceAccount:${google_service_account.trivy_operator_sa.email}"
}

# WIF binding for Trivy Operator. KSA `trivy-operator` in namespace
# `trivy-system` (Trivy Operator Helm release) — correct WI form.
resource "google_service_account_iam_member" "trivy_wif_binding" {
  service_account_id = google_service_account.trivy_operator_sa.id
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[trivy-system/trivy-operator]"
}
