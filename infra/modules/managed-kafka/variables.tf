# Managed Kafka module variables

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

variable "cluster_id" {
  description = "Kafka cluster ID"
  type        = string
}

variable "kafka_capacity" {
  description = "Kafka capacity configuration"
  type = object({
    vcpu_count    = number
    memory_bytes  = number
  })
  default = {
    vcpu_count   = 3  # Minimum per API
    memory_bytes = 12595200000  # ~12GB minimum per API
  }
}

variable "kafka_broker_count" {
  description = "Number of Kafka brokers (minimum 3 per GCP API)"
  type        = number
  default     = 3
  validation {
    condition     = var.kafka_broker_count >= 3
    error_message = "Managed Kafka requires at least 3 brokers"
  }
}

variable "topic_partitions" {
  description = "Default number of partitions per topic"
  type        = number
  default     = 6
}

variable "topic_replication_factor" {
  description = "Replication factor for topics"
  type        = number
  default     = 3
  validation {
    condition     = var.topic_replication_factor >= 1 && var.topic_replication_factor <= 3
    error_message = "Replication factor must be between 1 and 3"
  }
}

variable "topic_names" {
  description = "List of topic names to create"
  type        = list(string)
  default = [
    "task.run-requested",
    "task.review-requested",
    "task.stop-requested",
    "task.pr-open-requested",
    "step",
    "run.completed",
    "finding",
    "verdict",
    "pr.opened",
    "task.status-changed",
    "signup.requested",
    "signup.approved",
    "signup.declined",
    "invite.created",
    "workspace.created",
    "mcp.created",
    "mcp.deleted",
    "skill.created",
    "skill.deleted",
    "run.started",
    "audit.recorded"
  ]
}
