# WIF module outputs

output "wif_pool_id" {
  description = "Workload Identity Pool ID"
  value       = google_iam_workload_identity_pool.github_pool.workload_identity_pool_id
}

output "wif_pool_name" {
  description = "Workload Identity Pool name (full resource path)"
  value       = google_iam_workload_identity_pool.github_pool.name
}

output "wif_provider_id" {
  description = "Workload Identity Provider ID"
  value       = google_iam_workload_identity_pool_provider.github_provider.workload_identity_pool_provider_id
}

output "wif_provider_name" {
  description = "Workload Identity Provider name (full resource path)"
  value       = google_iam_workload_identity_pool_provider.github_provider.name
}

output "ci_sa_email" {
  description = "CI service account email"
  value       = google_service_account.ci_sa.email
}

output "ci_sa_name" {
  description = "CI service account resource name"
  value       = google_service_account.ci_sa.name
}

output "argocd_sa_email" {
  description = "Argo CD service account email"
  value       = google_service_account.argocd_sa.email
}

output "argocd_sa_name" {
  description = "Argo CD service account resource name"
  value       = google_service_account.argocd_sa.name
}

output "external_secrets_sa_email" {
  description = "External Secrets Operator service account email"
  value       = google_service_account.external_secrets_sa.email
}

output "external_secrets_sa_name" {
  description = "External Secrets Operator service account resource name"
  value       = google_service_account.external_secrets_sa.name
}

output "trivy_operator_sa_email" {
  description = "Trivy Operator service account email"
  value       = google_service_account.trivy_operator_sa.email
}

output "trivy_operator_sa_name" {
  description = "Trivy Operator service account resource name"
  value       = google_service_account.trivy_operator_sa.name
}

output "kubernetes_service_accounts" {
  description = "Map of Kubernetes service accounts to GCP service accounts for Workload Identity"
  value = {
    argocd = {
      k8s_sa = "argocd-system/argocd-server"
      gcp_sa = google_service_account.argocd_sa.email
    }
    external_secrets = {
      k8s_sa = "default/external-secrets-sa"
      gcp_sa = google_service_account.external_secrets_sa.email
    }
    trivy_operator = {
      k8s_sa = "trivy-system/trivy-operator"
      gcp_sa = google_service_account.trivy_operator_sa.email
    }
  }
}

output "github_actions_wif_provider" {
  description = "Full WIF provider string for GitHub Actions"
  value       = "${var.project_id}/locations/global/workloadIdentityPools/${local.pool_id}/providers/${local.provider_id}"
}
