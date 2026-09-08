# Filestore module variables

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
  description = "Zone suffix (a/b/c)"
  type        = string
  default     = "a"
}

variable "instance_name" {
  description = "Filestore instance name"
  type        = string
}

variable "network_id" {
  description = "VPC network ID"
  type        = string
}

variable "capacity_gb" {
  description = "Storage capacity in GB (min 1024, max 102400)"
  type        = number
  default     = 1024
  validation {
    condition     = var.capacity_gb >= 1024 && var.capacity_gb <= 102400
    error_message = "Capacity must be between 1024 and 102400 GB"
  }
}

variable "tier" {
  description = "Filestore tier (BASIC_HDD, BASIC_SSD, PREMIUM, HIGH_SCALE_SSD)"
  type        = string
  default     = "BASIC_HDD"
  validation {
    condition     = contains(["BASIC_HDD", "BASIC_SSD", "PREMIUM", "HIGH_SCALE_SSD"], var.tier)
    error_message = "Tier must be BASIC_HDD, BASIC_SSD, PREMIUM, or HIGH_SCALE_SSD"
  }
}
