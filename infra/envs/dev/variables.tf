# Dev environment variables

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "project_number" {
  description = "GCP project number"
  type        = number
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "zone_suffix" {
  description = "GCP zone suffix"
  type        = string
  default     = "a"
}

variable "github_owner" {
  description = "GitHub repository owner"
  type        = string
}

variable "github_repo" {
  description = "GitHub repository name"
  type        = string
}
