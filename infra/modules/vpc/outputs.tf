# VPC module outputs

output "vpc_id" {
  description = "VPC ID"
  value       = google_compute_network.vpc.id
}

output "vpc_name" {
  description = "VPC name"
  value       = google_compute_network.vpc.name
}

output "subnet_id" {
  description = "Subnet ID"
  value       = google_compute_subnetwork.subnet.id
}

output "subnet_name" {
  description = "Subnet name"
  value       = google_compute_subnetwork.subnet.name
}

output "subnet_region" {
  description = "Subnet region"
  value       = google_compute_subnetwork.subnet.region
}

output "pods_range_name" {
  description = "Pods secondary range name"
  value       = google_compute_subnetwork.subnet.secondary_ip_range[0].range_name
}

output "services_range_name" {
  description = "Services secondary range name"
  value       = google_compute_subnetwork.subnet.secondary_ip_range[1].range_name
}
