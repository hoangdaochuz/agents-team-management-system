# WIF module variables

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "environment" {
  description = "Environment name (dev/prod)"
  type        = string
}

variable "project_number" {
  description = "GCP project number"
  type        = number
}

variable "github_owner" {
  description = "GitHub repository owner (organization or user)"
  type        = string
}

variable "github_repo" {
  description = "GitHub repository name"
  type        = string
}

variable "artifact_registry_url" {
  description = "Artifact Registry URL for image push access"
  type        = string
}
