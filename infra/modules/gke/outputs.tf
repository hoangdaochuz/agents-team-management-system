# GKE module outputs

output "cluster_id" {
  description = "Cluster ID"
  value       = google_container_cluster.cluster.id
}

output "cluster_name" {
  description = "Cluster name"
  value       = google_container_cluster.cluster.name
}

output "cluster_endpoint" {
  description = "Cluster endpoint"
  value       = google_container_cluster.cluster.endpoint
  sensitive   = true
}

output "cluster_ca_certificate" {
  description = "Cluster CA certificate (base64)"
  value       = google_container_cluster.cluster.master_auth[0].cluster_ca_certificate
  sensitive   = true
}

output "cluster_location" {
  description = "Cluster location (region or zone)"
  value       = google_container_cluster.cluster.location
}

output "cluster_version" {
  description = "Cluster Kubernetes version"
  value       = google_container_cluster.cluster.master_version
}

output "node_pool_names" {
  description = "Node pool names"
  value = [
    google_container_node_pool.default_pool.name,
    google_container_node_pool.sandbox_pool.name
  ]
}

output "sandbox_taint" {
  description = "Taint configuration for sandbox node pool"
  value = {
    key    = "aaks/sandbox"
    value  = "true"
    effect = "NO_SCHEDULE"
  }
}

output "node_pool_service_account" {
  description = "Service account email for node pools"
  value       = google_service_account.default_pool_sa.email
}

output "master_ipv4_cidr_block" {
  description = "CIDR block for master access"
  value       = var.master_ipv4_cidr_block
}
