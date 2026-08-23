# GKE module variables

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

variable "zones" {
  description = "GCP zones for the cluster (empty for regional cluster in all zones)"
  type        = list(string)
  default     = null
}

variable "cluster_name" {
  description = "Name of the GKE cluster"
  type        = string
}

variable "network_id" {
  description = "VPC network ID"
  type        = string
}

variable "subnet_id" {
  description = "Subnetwork ID"
  type        = string
}

variable "pods_range_name" {
  description = "Secondary range name for pods"
  type        = string
}

variable "services_range_name" {
  description = "Secondary range name for services"
  type        = string
}

variable "master_ipv4_cidr_block" {
  description = "CIDR block for GKE master access"
  type        = string
  default     = "172.16.0.0/28"
}

variable "release_channel" {
  description = "GKE release channel"
  type        = string
  default     = "REGULAR"
  validation {
    condition     = contains(["RAPID", "REGULAR", "STABLE", "UNSPECIFIED"], var.release_channel)
    error_message = "Release channel must be one of: RAPID, REGULAR, STABLE, UNSPECIFIED"
  }
}

variable "security_posture_config" {
  description = "Security posture for the cluster"
  type        = string
  default     = "BASIC"
  validation {
    condition     = contains(["BASIC", "ENTERPRISE"], var.security_posture_config)
    error_message = "Security posture must be BASIC or ENTERPRISE"
  }
}

# Default node pool settings
variable "default_node_pool_machine_type" {
  description = "Machine type for default node pool"
  type        = string
  default     = "e2-standard-4"
}

variable "default_node_pool_min_nodes" {
  description = "Minimum nodes for default pool"
  type        = number
  default     = 1
}

variable "default_node_pool_max_nodes" {
  description = "Maximum nodes for default pool"
  type        = number
  default     = 3
}

variable "default_node_pool_initial_node_count" {
  description = "Initial node count for default pool"
  type        = number
  default     = 1
}

variable "default_node_pool_spot" {
  description = "Use spot VMs for default pool (dev only)"
  type        = bool
  default     = false
}

variable "default_node_pool_disk_size_gb" {
  description = "Disk size in GB for default pool nodes"
  type        = number
  default     = 100
}

variable "default_node_pool_disk_type" {
  description = "Disk type for default pool nodes"
  type        = string
  default     = "pd-balanced"
}

# Sandbox node pool settings
variable "sandbox_node_pool_machine_type" {
  description = "Machine type for sandbox node pool (DinD requires more resources)"
  type        = string
  default     = "e2-standard-4"
}

variable "sandbox_node_pool_min_nodes" {
  description = "Minimum nodes for sandbox pool"
  type        = number
  default     = 1
}

variable "sandbox_node_pool_max_nodes" {
  description = "Maximum nodes for sandbox pool"
  type        = number
  default     = 3
}

variable "sandbox_node_pool_initial_node_count" {
  description = "Initial node count for sandbox pool"
  type        = number
  default     = 1
}

variable "sandbox_node_pool_spot" {
  description = "Use spot VMs for sandbox pool"
  type        = bool
  default     = false
}

variable "sandbox_node_pool_disk_size_gb" {
  description = "Disk size in GB for sandbox pool nodes"
  type        = number
  default     = 200
}

variable "sandbox_node_pool_disk_type" {
  description = "Disk type for sandbox pool nodes"
  type        = string
  default     = "pd-balanced"
}
