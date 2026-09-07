# VPC Module - Creates VPC with subnets, Cloud NAT, and private access

resource "google_compute_network" "vpc" {
  name                            = var.vpc_name
  project                         = var.project_id
  description                     = "VPC for AAKS ${var.environment}"
  routing_mode                    = "REGIONAL"
  auto_create_subnetworks         = false
  delete_default_routes_on_create = false
}

# Primary subnet with secondary ranges for GKE
resource "google_compute_subnetwork" "subnet" {
  name          = "${var.vpc_name}-subnet"
  project       = var.project_id
  region        = var.region
  network       = google_compute_network.vpc.id
  ip_cidr_range = var.subnet_cidr
  description   = "Primary subnet for AAKS ${var.environment}"

  secondary_ip_range {
    range_name    = "pods"
    ip_cidr_range = var.pods_cidr
  }

  secondary_ip_range {
    range_name    = "services"
    ip_cidr_range = var.services_cidr
  }

  private_ip_google_access = true

  log_config {
    aggregation_interval = "INTERVAL_5_MIN"
    flow_sampling        = 0.5
    metadata             = "INCLUDE_ALL_METADATA"
  }
}

# Private Service Access: Cloud SQL (private IP) requires a peered service
# network. The reserved range + connection below are what
# `google_sql_database_instance.private_network` attaches to — without them
# instance creation fails.
resource "google_compute_global_address" "private_service_range" {
  name          = "${var.vpc_name}-psa-range"
  project       = var.project_id
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.vpc.id
}

resource "google_service_networking_connection" "private_service_access" {
  network                 = google_compute_network.vpc.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_service_range.name]
}

# Explicit firewall rules (default-deny ingress is implicit on GCP; these are
# the allows the platform needs — spec requires firewall in Terraform).
resource "google_compute_firewall" "allow_internal" {
  name    = "${var.vpc_name}-allow-internal"
  project = var.project_id
  network = google_compute_network.vpc.name

  direction = "INGRESS"
  priority  = 1000

  source_ranges = [var.subnet_cidr, var.pods_cidr, var.services_cidr]

  allow {
    protocol = "tcp"
    ports    = ["0-65535"]
  }
  allow {
    protocol = "udp"
    ports    = ["0-65535"]
  }
  allow {
    protocol = "icmp"
  }

  description = "Allow all internal traffic within the VPC and GKE ranges"
}

# GCP load-balancer + health-check ranges (GCE Ingress NEGs + Cloud SQL/Kafka
# health checks originate here).
resource "google_compute_firewall" "allow_gcp_lb_healthchecks" {
  name    = "${var.vpc_name}-allow-gcp-lb"
  project = var.project_id
  network = google_compute_network.vpc.name

  direction = "INGRESS"
  priority  = 1000

  source_ranges = ["130.211.0.0/22", "35.191.0.0/16"]

  allow {
    protocol = "tcp"
  }

  description = "Allow GCP load balancers and health checkers"
}

# IAP for Identity-Aware Proxy TCP forwarding (operator SSH/kubectl via
# `gcloud compute start-iap-tunnel` without public bastions).
resource "google_compute_firewall" "allow_iap" {
  name    = "${var.vpc_name}-allow-iap"
  project = var.project_id
  network = google_compute_network.vpc.name

  direction = "INGRESS"
  priority  = 1000

  source_ranges = ["35.235.240.0/20"]

  allow {
    protocol = "tcp"
    ports    = ["22", "443"]
  }

  description = "Allow IAP TCP forwarding for operator access"
}

# Cloud Router for NAT
resource "google_compute_router" "router" {
  name    = "${var.vpc_name}-router"
  project = var.project_id
  region  = var.region
  network = google_compute_network.vpc.id

  bgp {
    asn = 65000
  }
}

# Static egress IP (prod): IP-allowlisted externals need a stable source.
# Dev stays AUTO_ONLY (ephemeral) to save the reservation cost.
resource "google_compute_address" "nat" {
  count   = var.nat_static_ip ? 1 : 0
  name    = "${var.vpc_name}-nat-ip"
  project = var.project_id
  region  = var.region
}

# Cloud NAT for private nodes to reach external services
resource "google_compute_router_nat" "nat" {
  name                               = "${var.vpc_name}-nat"
  project                            = var.project_id
  router                             = google_compute_router.router.name
  region                             = var.region
  nat_ip_allocate_option             = var.nat_static_ip ? "MANUAL_ONLY" : "AUTO_ONLY"
  nat_ips                            = var.nat_static_ip ? [google_compute_address.nat[0].self_link] : null
  source_subnetwork_ip_ranges_to_nat = "LIST_OF_SUBNETWORKS"

  subnetwork {
    name                    = google_compute_subnetwork.subnet.id
    source_ip_ranges_to_nat = ["ALL_IP_RANGES"]
  }

  log_config {
    enable = true
    filter = "ERRORS_ONLY"
  }
}
