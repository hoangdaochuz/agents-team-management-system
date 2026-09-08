# Secrets Module - Secret Manager secret containers (values bootstrapped manually)
#
# Every secret_id carries the environment suffix: Secret Manager IDs are
# project-global, so unsuffixed dev+prod sharing one project would collide on
# the second `apply`. ExternalSecrets reference them via the `aaks-ENV-...`
# placeholder (replaced with the env at bootstrap).

# Agent master key - for encrypting LLM provider keys at rest (was the
# Settings service's SETTINGS_MASTER_KEY; the settings plane merged into the
# agent service in consolidate-microservices)
resource "google_secret_manager_secret" "agent_master_key" {
  project   = var.project_id
  secret_id = "aaks-${var.environment}-agent-master-key"
  replication {
    auto {}
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
    purpose     = "encryption"
  }
}

# Shared internal service token (INTERNAL_TOKEN, consumed by every service)
resource "google_secret_manager_secret" "internal_token" {
  project   = var.project_id
  secret_id = "aaks-${var.environment}-internal-token"
  replication {
    auto {}
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
    purpose     = "internal-auth"
  }
}

# Agent service internal token (AGENT_INTERNAL_TOKEN — guards the agent
# service's internal key-decrypt / MCP-hydration endpoints; consumed by the
# executor)
resource "google_secret_manager_secret" "agent_internal_token" {
  project   = var.project_id
  secret_id = "aaks-${var.environment}-agent-internal-token"
  replication {
    auto {}
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
    purpose     = "internal-auth"
  }
}

# Per-service database DSNs (values composed from the Cloud SQL private IP +
# db name and bootstrapped with the DB password; referenced by the
# identity/workspace/agent/executor-dsn ExternalSecrets)
resource "google_secret_manager_secret" "db_dsns" {
  for_each = toset([
    "aaks-${var.environment}-identity-db-dsn",
    "aaks-${var.environment}-workspace-db-dsn",
    "aaks-${var.environment}-agent-db-dsn",
    "aaks-${var.environment}-executor-db-dsn"
  ])
  project   = var.project_id
  secret_id = each.key
  replication {
    auto {}
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
    purpose     = "database"
  }
}

# Seed superadmin email
resource "google_secret_manager_secret" "auth_seed_superadmin_email" {
  project   = var.project_id
  secret_id = "aaks-${var.environment}-auth-seed-superadmin-email"
  replication {
    auto {}
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
    purpose     = "auth-seed"
  }
}

# Seed superadmin password
resource "google_secret_manager_secret" "auth_seed_superadmin_password" {
  project   = var.project_id
  secret_id = "aaks-${var.environment}-auth-seed-superadmin-password"
  replication {
    auto {}
  }

  # NOTE: no `rotation` block — provider v6 requires rotation `topics`
  # (Pub/Sub) alongside it, which we don't provision. Rotate with
  # `gcloud secrets versions add` (see infra/README.md secret bootstrap).

  labels = {
    environment = var.environment
    managed-by  = "terraform"
    purpose     = "auth-seed"
  }
}
