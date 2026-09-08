# VPC module variables

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

variable "vpc_name" {
  description = "Name of the VPC"
  type        = string
}

variable "subnet_cidr" {
  description = "CIDR range for the primary subnet"
  type        = string
  default     = "10.0.0.0/24"
}

variable "pods_cidr" {
  description = "CIDR range for GKE pods (secondary subnet range)"
  type        = string
  default     = "10.1.0.0/16"
}

variable "services_cidr" {
  description = "CIDR range for GKE services (secondary subnet range)"
  type        = string
  default     = "10.2.0.0/16"
}

variable "nat_static_ip" {
  description = "Reserve a static egress IP for Cloud NAT (prod needs stable source IPs for allowlisted externals)"
  type        = bool
  default     = false
}
