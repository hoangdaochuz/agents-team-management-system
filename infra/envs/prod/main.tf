# Prod environment - main configuration

provider "google" {
  project = var.project_id
  region  = var.region
}

# VPC Module
module "vpc" {
  source = "../../modules/vpc"

  project_id    = var.project_id
  environment   = "prod"
  region        = var.region
  vpc_name      = "aaks-prod"
  subnet_cidr   = "10.10.0.0/24"
  pods_cidr     = "10.11.0.0/16"
  services_cidr = "10.12.0.0/16"
  nat_static_ip = true
}

# Artifact Registry Module
module "artifact_registry" {
  source = "../../modules/artifact-registry"

  project_id     = var.project_id
  environment    = "prod"
  region         = var.region
  registry_name  = "aaks"
  immutable_tags = true
}

# GKE Module
module "gke" {
  source = "../../modules/gke"

  project_id   = var.project_id
  environment  = "prod"
  region       = var.region
  zones        = null # Regional cluster (all zones in region)
  cluster_name = "aaks-prod"

  network_id          = module.vpc.vpc_id
  subnet_id           = module.vpc.subnet_id
  pods_range_name     = module.vpc.pods_range_name
  services_range_name = module.vpc.services_range_name

  # Prod: larger sizing, no spot VMs for HA
  default_node_pool_machine_type       = "e2-standard-8"
  default_node_pool_min_nodes          = 3
  default_node_pool_max_nodes          = 10
  default_node_pool_initial_node_count = 3
  default_node_pool_spot               = false # No spot in prod
  default_node_pool_disk_size_gb       = 200
  default_node_pool_disk_type          = "pd-ssd"

  sandbox_node_pool_machine_type       = "e2-standard-8"
  sandbox_node_pool_min_nodes          = 2
  sandbox_node_pool_max_nodes          = 5
  sandbox_node_pool_initial_node_count = 2
  sandbox_node_pool_spot               = false # No spot in prod
  sandbox_node_pool_disk_size_gb       = 300
  sandbox_node_pool_disk_type          = "pd-ssd"
}

# Cloud SQL Module
module "cloudsql" {
  source = "../../modules/cloudsql"

  project_id  = var.project_id
  environment = "prod"
  region      = var.region
  zone_suffix = "a" # Primary zone
  network_id  = module.vpc.vpc_id

  db_name_prefix       = "aaks"
  db_tier              = "db-custom-4-15360" # 4 vCPU, 15GB RAM
  db_disk_size_gb      = 500
  db_disk_type         = "PD_SSD"
  db_availability_type = "REGIONAL" # HA for prod
  deletion_protection  = true

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
  environment = "prod"
  region      = var.region
  cluster_id  = "aaks"
  subnet_id   = module.vpc.subnet_id

  # Prod: larger capacity for production traffic
  kafka_capacity = {
    vcpu_count   = 9
    memory_bytes = 32212254720 # ~32GB
  }
  topic_partitions         = 12 # Higher partitions for prod
  topic_replication_factor = 3
}

# Filestore Module
module "filestore" {
  source = "../../modules/filestore"

  project_id    = var.project_id
  environment   = "prod"
  region        = var.region
  zone_suffix   = "a"
  instance_name = "aaks-clone-root"
  network_id    = module.vpc.vpc_id

  capacity_gb = 2048        # 2TB for prod
  tier        = "BASIC_SSD" # SSD for prod performance
}

# Secrets Module
module "secrets" {
  source = "../../modules/secrets"

  project_id  = var.project_id
  environment = "prod"
}

# WIF Module
module "wif" {
  source = "../../modules/wif"

  project_id   = var.project_id
  environment  = "prod"
  github_owner = var.github_owner
  github_repo  = var.github_repo
}
