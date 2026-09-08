# Dev environment - main configuration

provider "google" {
  project = var.project_id
  region  = var.region
}

# VPC Module
module "vpc" {
  source = "../../modules/vpc"

  project_id    = var.project_id
  environment   = "dev"
  region        = var.region
  vpc_name      = "aaks-dev"
  subnet_cidr   = "10.0.0.0/24"
  pods_cidr     = "10.1.0.0/16"
  services_cidr = "10.2.0.0/16"
}

# Artifact Registry Module
module "artifact_registry" {
  source = "../../modules/artifact-registry"

  project_id    = var.project_id
  environment   = "dev"
  region        = var.region
  registry_name = "aaks"
}

# GKE Module
module "gke" {
  source = "../../modules/gke"

  project_id   = var.project_id
  environment  = "dev"
  region       = var.region
  zones        = ["${var.region}-${var.zone_suffix}"] # Zonal cluster
  cluster_name = "aaks-dev"

  network_id          = module.vpc.vpc_id
  subnet_id           = module.vpc.subnet_id
  pods_range_name     = module.vpc.pods_range_name
  services_range_name = module.vpc.services_range_name

  # Dev: small sizing, spot VMs for default pool
  default_node_pool_machine_type       = "e2-standard-4"
  default_node_pool_min_nodes          = 1
  default_node_pool_max_nodes          = 3
  default_node_pool_initial_node_count = 1
  default_node_pool_spot               = true
  default_node_pool_disk_size_gb       = 100
  default_node_pool_disk_type          = "pd-balanced"

  sandbox_node_pool_machine_type       = "e2-standard-4"
  sandbox_node_pool_min_nodes          = 1
  sandbox_node_pool_max_nodes          = 3
  sandbox_node_pool_initial_node_count = 1
  sandbox_node_pool_spot               = false # Sandbox needs stable capacity
  sandbox_node_pool_disk_size_gb       = 200
  sandbox_node_pool_disk_type          = "pd-balanced"
}

# Cloud SQL Module
module "cloudsql" {
  source = "../../modules/cloudsql"

  project_id  = var.project_id
  environment = "dev"
  region      = var.region
  zone_suffix = var.zone_suffix
  network_id  = module.vpc.vpc_id

  db_name_prefix       = "aaks"
  db_tier              = "db-g1-small"
  db_disk_size_gb      = 100
  db_disk_type         = "PD_SSD"
  db_availability_type = "ZONAL"
  deletion_protection  = false # dev iterates via destroy; prod keeps true

  backup_enabled    = true
  backup_start_time = "03:00"

  # The private-IP instance needs the VPC's Private Service Access peering
  # first — order explicitly (network_id alone doesn't imply it).
  depends_on = [module.vpc]
}

# Managed Kafka Module
module "managed_kafka" {
  source = "../../modules/managed-kafka"

  project_id  = var.project_id
  environment = "dev"
  region      = var.region
  cluster_id  = "aaks"
  subnet_id   = module.vpc.subnet_id

  # Dev: smallest legal capacity (minimum 3 vCPU, ~12GB memory per broker)
  kafka_capacity = {
    vcpu_count   = 3
    memory_bytes = 12595200000 # ~12GB
  }
  topic_partitions         = 6
  topic_replication_factor = 3
}

# Filestore Module
module "filestore" {
  source = "../../modules/filestore"

  project_id    = var.project_id
  environment   = "dev"
  region        = var.region
  zone_suffix   = var.zone_suffix
  instance_name = "aaks-clone-root"
  network_id    = module.vpc.vpc_id

  capacity_gb = 1024        # 1TB minimum
  tier        = "BASIC_HDD" # Dev: HDD for cost savings
}

# Secrets Module
module "secrets" {
  source = "../../modules/secrets"

  project_id  = var.project_id
  environment = "dev"
}

# WIF Module
module "wif" {
  source = "../../modules/wif"

  project_id   = var.project_id
  environment  = "dev"
  github_owner = var.github_owner
  github_repo  = var.github_repo
}
