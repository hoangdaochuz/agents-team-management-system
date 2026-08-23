# Secrets Module - Secret Manager secret containers (values bootstrapped manually)

# Settings master key - for encrypting LLM provider keys at rest
resource "google_secret_manager_secret" "settings_master_key" {
  project      = var.project_id
  secret_id    = "settings-master-key"
  replication_policy {
    automatic = true
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
    purpose     = "encryption"
  }
}

# Internal service token - for mTLS+token auth between services
resource "google_secret_manager_secret" "settings_internal_token" {
  project      = var.project_id
  secret_id    = "settings-internal-token"
  replication_policy {
    automatic = true
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
    purpose     = "internal-auth"
  }
}

# Seed superadmin email
resource "google_secret_manager_secret" "auth_seed_superadmin_email" {
  project      = var.project_id
  secret_id    = "auth-seed-superadmin-email"
  replication_policy {
    automatic = true
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
    purpose     = "auth-seed"
  }
}

# Seed superadmin password
resource "google_secret_manager_secret" "auth_seed_superadmin_password" {
  project      = var.project_id
  secret_id    = "auth-seed-superadmin-password"
  replication_policy {
    automatic = true
  }

  # Enable automatic secret rotation every 90 days
  automatic {
    replication_policy {
      automatic = true
    }
  }

  rotation {
    rotation_period = "7776000s" # 90 days
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
    purpose     = "auth-seed"
  }
}
