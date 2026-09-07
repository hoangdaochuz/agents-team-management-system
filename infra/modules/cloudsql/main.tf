# Cloud SQL Module - PostgreSQL instance with 4 logical databases

locals {
  db_instance_name = "${var.db_name_prefix}-${var.environment}"
  full_zone        = "${var.region}-${var.zone_suffix}"
}

# Random password for the database user
resource "random_password" "db_password" {
  length = 32
  # No special characters: the password is composed into `postgres://` DSNs
  # (see infra/README.md) and most specials would need URL-encoding. 32
  # alphanumeric chars is ~190 bits — plenty.
  special = false
  upper   = true
  numeric = true
}

# Store the password in Secret Manager immediately
resource "google_secret_manager_secret" "db_password" {
  project   = var.project_id
  secret_id = "${local.db_instance_name}-db-password"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "db_password" {
  secret      = google_secret_manager_secret.db_password.id
  secret_data = random_password.db_password.result
}

# Cloud SQL PostgreSQL instance
resource "google_sql_database_instance" "instance" {
  name                = local.db_instance_name
  project             = var.project_id
  region              = var.region
  database_version    = "POSTGRES_16"
  deletion_protection = var.deletion_protection

  settings {
    tier              = var.db_tier
    disk_size         = var.db_disk_size_gb
    disk_type         = var.db_disk_type
    activation_policy = "ALWAYS"

    # Availability configuration
    availability_type = var.db_availability_type

    # Backup configuration
    backup_configuration {
      enabled                        = var.backup_enabled
      start_time                     = var.backup_start_time
      transaction_log_retention_days = 7
      point_in_time_recovery_enabled = true
    }

    # Location preference
    location_preference {
      zone = local.full_zone
    }

    # Maintenance window
    maintenance_window {
      day          = 7 # Sunday
      hour         = 3
      update_track = "stable"
    }

    # Database flags
    database_flags {
      name  = "max_connections"
      value = "200"
    }
    database_flags {
      name  = "shared_buffers"
      value = "256MB"
    }
    database_flags {
      name  = "effective_cache_size"
      value = "1GB"
    }
    database_flags {
      name  = "log_statement"
      value = "ddl" # `all` drowns Cloud Logging (and cost) in prod; DDL keeps schema-change audit
    }

    # IP configuration - private only, encrypted only (provider v6 renamed
    # the old `require_ssl` flag to `ssl_mode`; ENCRYPTED_ONLY matches the
    # `?sslmode=require` DSNs the services are bootstrapped with).
    ip_configuration {
      ipv4_enabled    = false
      private_network = var.network_id
      ssl_mode        = "ENCRYPTED_ONLY"

      # NOTE: no `authorized_networks` — ipv4_enabled=false makes them dead
      # config, and 10.0.0.0/8 would be over-broad anyway. Private IP only.
    }

    # User flags
    user_labels = {
      environment = var.environment
      managed-by  = "terraform"
    }
  }

  timeouts {
    create = "30m"
    update = "30m"
    delete = "30m"
  }

  depends_on = [google_secret_manager_secret_version.db_password]
}

# Create the logical databases using for_each
resource "google_sql_database" "databases" {
  for_each = toset(var.database_names)

  project   = var.project_id
  instance  = google_sql_database_instance.instance.name
  name      = each.value
  charset   = "UTF8"
  collation = "en_US.UTF8"

  depends_on = [google_secret_manager_secret_version.db_password]
}

# Create database user (aaks) with the generated password
resource "google_sql_user" "aaks_user" {
  project  = var.project_id
  instance = google_sql_database_instance.instance.name
  name     = "aaks"
  password = random_password.db_password.result

  depends_on = [google_secret_manager_secret_version.db_password]
}
