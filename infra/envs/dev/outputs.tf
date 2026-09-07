# Dev environment outputs

# Cluster outputs
output "cluster_endpoint" {
  description = "GKE cluster endpoint"
  value       = module.gke.cluster_endpoint
  sensitive   = true
}

output "cluster_ca_certificate" {
  description = "GKE cluster CA certificate"
  value       = module.gke.cluster_ca_certificate
  sensitive   = true
}

output "cluster_name" {
  description = "GKE cluster name"
  value       = module.gke.cluster_name
}

output "sandbox_taint" {
  description = "Sandbox node pool taint configuration"
  value       = module.gke.sandbox_taint
}

# Data service outputs
output "sql_connection_name" {
  description = "Cloud SQL connection name"
  value       = module.cloudsql.instance_connection_name
}

output "sql_instance_name" {
  description = "Cloud SQL instance name"
  value       = module.cloudsql.instance_name
}

output "kafka_bootstrap_servers" {
  description = "Kafka bootstrap servers"
  value       = module.managed_kafka.bootstrap_servers
}

output "kafka_topic_names" {
  description = "Kafka topic names"
  value       = module.managed_kafka.topic_names
}

output "filestore_mount_target" {
  description = "Filestore NFS mount target"
  value       = module.filestore.mount_target
}

# Registry outputs
output "registry_url" {
  description = "Artifact Registry URL"
  value       = module.artifact_registry.registry_url
}

# Secret outputs
output "db_password_secret_name" {
  description = "Cloud SQL password secret name"
  value       = module.cloudsql.db_user_secret_name
}

output "kafka_username_secret_name" {
  description = "Kafka username secret name"
  value       = module.managed_kafka.username_secret_name
}

output "kafka_password_secret_name" {
  description = "Kafka password secret name"
  value       = module.managed_kafka.password_secret_name
}

output "secret_names" {
  description = "All secret names"
  value = {
    agent_master_key          = module.secrets.agent_master_key_secret_name
    internal_token            = module.secrets.internal_token_secret_name
    agent_internal_token      = module.secrets.agent_internal_token_secret_name
    db_dsns                   = module.secrets.db_dsn_secret_names
    auth_seed_superadmin_email = module.secrets.auth_seed_superadmin_email_secret_name
    auth_seed_superadmin_password = module.secrets.auth_seed_superadmin_password_secret_name
    db_password               = module.cloudsql.db_user_secret_name
    kafka_username            = module.managed_kafka.username_secret_name
    kafka_password            = module.managed_kafka.password_secret_name
  }
}

# WIF outputs
output "wif_provider" {
  description = "GitHub Actions WIF provider string"
  value       = module.wif.github_actions_wif_provider
}

output "ci_sa_email" {
  description = "CI service account email"
  value       = module.wif.ci_sa_email
}

output "kubernetes_service_accounts" {
  description = "Kubernetes to GCP service account mappings"
  value       = module.wif.kubernetes_service_accounts
}

# VPC outputs
output "vpc_name" {
  description = "VPC name"
  value       = module.vpc.vpc_name
}

output "subnet_name" {
  description = "Subnet name"
  value       = module.vpc.subnet_name
}
