# Artifact Registry Module - Docker image repository

resource "google_artifact_registry_repository" "registry" {
  location      = var.region
  repository_id = "${var.registry_name}-${var.environment}"
  description   = "Docker registry for AAKS ${var.environment}"
  format        = "DOCKER"
  project       = var.project_id

  docker_config {
    immutable_tags = var.immutable_tags
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
  }
}
