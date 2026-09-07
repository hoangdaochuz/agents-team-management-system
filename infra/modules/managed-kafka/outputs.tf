# Managed Kafka module outputs

output "cluster_id" {
  description = "Kafka cluster ID"
  value       = google_managed_kafka_cluster.cluster.cluster_id
}

output "cluster_name" {
  description = "Kafka cluster name"
  value       = google_managed_kafka_cluster.cluster.cluster_id
}

output "cluster_location" {
  description = "Kafka cluster location"
  value       = google_managed_kafka_cluster.cluster.location
}

output "bootstrap_lookup" {
  description = "Command resolving the real bootstrap endpoints (Managed Kafka exposes no endpoint attribute in Terraform) — paste the result into the k8s env-values `kafka-brokers`"
  value       = "gcloud managed-kafka clusters describe ${google_managed_kafka_cluster.cluster.cluster_id} --location=${google_managed_kafka_cluster.cluster.location} --project=${var.project_id}"
}

output "topic_names" {
  description = "List of created topic names (built from the actual resources, not the input list)"
  value       = concat(values(google_managed_kafka_topic.topics)[*].topic_id, [google_managed_kafka_topic.consumer_offsets.topic_id])
}

output "username_secret_name" {
  description = "Secret Manager secret name for Kafka username"
  value       = google_secret_manager_secret.kafka_username.secret_id
}

output "username_secret_id" {
  description = "Secret Manager secret resource ID for username"
  value       = google_secret_manager_secret.kafka_username.id
}

output "password_secret_name" {
  description = "Secret Manager secret name for Kafka password"
  value       = google_secret_manager_secret.kafka_password.secret_id
}

output "password_secret_id" {
  description = "Secret Manager secret resource ID for password"
  value       = google_secret_manager_secret.kafka_password.id
}

output "sasl_mechanism" {
  description = "SASL mechanism"
  value       = "PLAIN"
}

output "security_protocol" {
  description = "Security protocol (TLS-only brokers: SASL/PLAIN runs over TLS, i.e. SASL_SSL)"
  value       = "SASL_SSL"
}
