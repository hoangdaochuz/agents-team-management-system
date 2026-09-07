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

output "bootstrap_servers" {
  description = "Kafka bootstrap server endpoints"
  value = [
    "${google_managed_kafka_cluster.cluster.cluster_id}-${var.kafka_broker_count - 1}.${var.region}.kafka.gcloud.internal:9092",
  ]
}

output "topic_names" {
  description = "List of created topic names"
  value       = concat(values(google_managed_kafka_topic.topics)[*].topic_id, ["__consumer_offsets"])
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
  description = "Security protocol"
  value       = "SASL_PLAINTEXT"
}
