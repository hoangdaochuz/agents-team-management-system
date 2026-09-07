# Artifact Registry module variables

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "environment" {
  description = "Environment name (dev/prod)"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "registry_name" {
  description = "Name of the artifact registry repository"
  type        = string
  default     = "aaks"
}
