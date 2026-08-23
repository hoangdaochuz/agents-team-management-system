# Cloud SQL module variables

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

variable "zone_suffix" {
  description = "Zone suffix (a/b/c) for primary instance"
  type        = string
  default     = "a"
}

variable "network_id" {
  description = "VPC network ID for private IP"
  type        = string
}

variable "db_name_prefix" {
  description = "Prefix for database names"
  type        = string
  default     = "aaks"
}

variable "db_tier" {
  description = "Database tier (e.g., db-g1-small, db-custom-2-7680)"
  type        = string
  default     = "db-g1-small"
}

variable "db_disk_size_gb" {
  description = "Disk size in GB"
  type        = number
  default     = 100
}

variable "db_disk_type" {
  description = "Disk type (PD_HDD, PD_SSD)"
  type        = string
  default     = "PD_SSD"
  validation {
    condition     = contains(["PD_HDD", "PD_SSD"], var.db_disk_type)
    error_message = "Disk type must be PD_HDD or PD_SSD"
  }
}

variable "db_availability_type" {
  description = "Availability type (ZONAL, REGIONAL)"
  type        = string
  default     = "ZONAL"
  validation {
    condition     = contains(["ZONAL", "REGIONAL"], var.db_availability_type)
    error_message = "Availability type must be ZONAL or REGIONAL"
  }
}

variable "deletion_protection" {
  description = "Enable deletion protection"
  type        = bool
  default     = true
}

variable "database_names" {
  description = "List of logical database names to create"
  type        = list(string)
  default     = [
    "project_db",
    "task_db",
    "agent_db",
    "catalog_db",
    "settings_db",
    "runner_db",
    "auth_db",
    "orgs_db",
    "resources_db",
    "admin_db"
  ]
}

variable "backup_enabled" {
  description = "Enable automated backups"
  type        = bool
  default     = true
}

variable "backup_start_time" {
  description = "Backup start time (HH:MM format)"
  type        = string
  default     = "03:00"
}
