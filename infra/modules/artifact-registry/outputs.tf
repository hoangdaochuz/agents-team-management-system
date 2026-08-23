# Artifact Registry module outputs

output "registry_id" {
  description = "Registry repository ID"
  value       = google_artifact_registry_repository.registry.repository_id
}

output "registry_name" {
  description = "Registry repository name"
  value       = google_artifact_registry_repository.registry.name
}

output "registry_url" {
  description = "Full Docker registry URL"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${var.registry_name}-${var.environment}"
}
