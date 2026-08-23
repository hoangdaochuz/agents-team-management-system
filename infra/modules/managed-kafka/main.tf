# Managed Kafka Module - Google Cloud Managed Service for Apache Kafka

locals {
  kafka_cluster_name = "${var.cluster_id}-${var.environment}"
  # Consumers publish poison messages to <topic>.dlq (platform/kafka DLQTopic).
  # Managed Kafka has no auto-create, so the DLQ topics must exist explicitly.
  all_topics = concat(var.topic_names, [for t in var.topic_names : "${t}.dlq"])
}

# Random credentials for SASL/PLAIN authentication
resource "random_password" "kafka_username" {
  length  = 16
  special = false
  upper   = false
  numeric  = false
}

resource "random_password" "kafka_password" {
  length  = 32
  special = true
  upper   = true
  numeric  = true
}

# Store Kafka credentials in Secret Manager. Names must match the
# kafka-credentials ExternalSecret (k8s/components/external-secrets).
resource "google_secret_manager_secret" "kafka_username" {
  project      = var.project_id
  secret_id    = "aaks-${var.environment}-kafka-sasl-user"
  replication_policy {
    automatic = true
  }
}

resource "google_secret_manager_secret_version" "kafka_username" {
  secret      = google_secret_manager_secret.kafka_username.id
  secret_data = random_password.kafka_username.result
}

resource "google_secret_manager_secret" "kafka_password" {
  project      = var.project_id
  secret_id    = "aaks-${var.environment}-kafka-sasl-password"
  replication_policy {
    automatic = true
  }
}

resource "google_secret_manager_secret_version" "kafka_password" {
  secret      = google_secret_manager_secret.kafka_password.id
  secret_data = random_password.kafka_password.result
}

# Managed Kafka Cluster
resource "google_managed_kafka_cluster" "cluster" {
  cluster_id   = local.kafka_cluster_name
  project      = var.project_id
  location     = var.region

  capacity_config {
    vcpu_count   = var.kafka_capacity.vcpu_count
    memory_bytes = var.kafka_capacity.memory_bytes
  }

  rebalance_config {
    mode = "ADD_ONLY"
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
  }

  timeouts {
    create = "60m"
    update = "60m"
    delete = "60m"
  }

  depends_on = [
    google_secret_manager_secret_version.kafka_username,
    google_secret_manager_secret_version.kafka_password
  ]
}

# Kafka topics - all system topics plus their DLQ twins
resource "google_managed_kafka_topic" "topics" {
  for_each = toset(local.all_topics)

  project     = var.project_id
  cluster_id  = google_managed_kafka_cluster.cluster.cluster_id
  location    = var.region
  topic_id    = each.value
  partitions  = var.topic_partitions

  # Special config for __consumer_offsets
  replication_factor = each.value == "__consumer_offsets" ? 1 : var.topic_replication_factor

  # Cleanup policies
  dynamic "config" {
    for_each = each.value == "__consumer_offsets" ? [1] : []
    content {
      key   = "cleanup.policy"
      value = "compact"
    }
  }

  # Increase retention for step topic (realtime streaming)
  dynamic "config" {
    for_each = each.value == "step" ? [1] : []
    content {
      key   = "retention.ms"
      value = "86400000"  # 24 hours
    }
  }

  # Retention for audit topics (longer)
  dynamic "config" {
    for_each = contains(["audit.recorded", "task.status-changed"], each.value) ? [1] : []
    content {
      key   = "retention.ms"
      value = "604800000"  # 7 days
    }
  }

  depends_on = [google_secret_manager_secret_version.kafka_password]
}

# Create __consumer_offsets topic explicitly if not in the list
resource "google_managed_kafka_topic" "consumer_offsets" {
  project     = var.project_id
  cluster_id  = google_managed_kafka_cluster.cluster.cluster_id
  location    = var.region
  topic_id    = "__consumer_offsets"
  partitions  = 50
  replication_factor = 1

  config {
    key   = "cleanup.policy"
    value = "compact"
  }

  lifecycle {
    create_before_destroy = true
  }
}
