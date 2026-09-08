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
  numeric = false
}

resource "random_password" "kafka_password" {
  length = 32
  # Alphanumeric only: SASL passwords travel in env + Secret Manager without
  # quoting surprises. (The username below already uses special=false.)
  special = false
  upper   = true
  numeric = true
}

# Store Kafka credentials in Secret Manager. Names must match the
# kafka-credentials ExternalSecret (k8s/components/external-secrets).
resource "google_secret_manager_secret" "kafka_username" {
  project   = var.project_id
  secret_id = "aaks-${var.environment}-kafka-sasl-user"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "kafka_username" {
  secret      = google_secret_manager_secret.kafka_username.id
  secret_data = random_password.kafka_username.result
}

resource "google_secret_manager_secret" "kafka_password" {
  project   = var.project_id
  secret_id = "aaks-${var.environment}-kafka-sasl-password"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "kafka_password" {
  secret      = google_secret_manager_secret.kafka_password.id
  secret_data = random_password.kafka_password.result
}

# Managed Kafka Cluster (private access into the VPC via `gcp_config` — the
# brokers get no public IPs; services reach them over the VPC).
resource "google_managed_kafka_cluster" "cluster" {
  cluster_id = local.kafka_cluster_name
  project    = var.project_id
  location   = var.region

  capacity_config {
    vcpu_count   = var.kafka_capacity.vcpu_count
    memory_bytes = var.kafka_capacity.memory_bytes
  }

  gcp_config {
    access_config {
      network_configs {
        subnet = var.subnet_id
      }
    }
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

  project         = var.project_id
  cluster         = google_managed_kafka_cluster.cluster.cluster_id
  location        = var.region
  topic_id        = each.value
  partition_count = var.topic_partitions

  replication_factor = var.topic_replication_factor

  # Per-topic Kafka configs (provider v6 `configs` map — not nested blocks).
  configs = merge(
    each.value == "__consumer_offsets" ? { "cleanup.policy" = "compact" } : {},
    # Increase retention for step topic (realtime streaming, 24 hours)
    each.value == "step" ? { "retention.ms" = "86400000" } : {},
  )

  depends_on = [google_secret_manager_secret_version.kafka_password]
}

# __consumer_offsets is provisioned explicitly (it is not in topic_names, so
# it has its own resource): RF=3 like everything else — a single-replica
# offsets topic would stall every consumer group if its broker hiccups.
resource "google_managed_kafka_topic" "consumer_offsets" {
  project            = var.project_id
  cluster            = google_managed_kafka_cluster.cluster.cluster_id
  location           = var.region
  topic_id           = "__consumer_offsets"
  partition_count    = 50
  replication_factor = 3

  configs = {
    "cleanup.policy" = "compact"
  }

  lifecycle {
    create_before_destroy = true
  }
}
