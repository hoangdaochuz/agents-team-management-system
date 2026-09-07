# GKE Module - Standard cluster with default and sandbox node pools

locals {
  # Determine if this is a zonal (single-zone) or regional (multi-zone) cluster
  is_zonal       = var.zones != null && length(var.zones) == 1
  node_locations = local.is_zonal ? var.zones : null
}

resource "google_container_cluster" "cluster" {
  name     = var.cluster_name
  project  = var.project_id
  location = local.is_zonal ? var.zones[0] : var.region

  network    = var.network_id
  subnetwork = var.subnet_id

  # Private cluster configuration
  private_cluster_config {
    enable_private_endpoint = true
    master_ipv4_cidr_block  = var.master_ipv4_cidr_block
  }

  # Master authorized networks (provider v6: top-level block, NOT nested in
  # private_cluster_config). Empty in prod = no external master access.
  master_authorized_networks_config {
    dynamic "cidr_blocks" {
      for_each = var.environment == "dev" ? [1] : []
      content {
        cidr_block   = "0.0.0.0/0"
        display_name = "Allow all for dev (restrict in production)"
      }
    }
  }

  # Release channel (provider v6: block, not a bare argument)
  release_channel {
    channel = var.release_channel
  }

  # Security posture
  security_posture_config {
    mode = var.security_posture_config
  }

  # Workload Identity
  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  # Pod Security Policy is long removed upstream (and superseded by the Pod
  # Security Standards labels in k8s/) — provider v6 dropped the block, so
  # there is nothing to declare here.

  # Network policy
  network_policy {
    enabled  = true
    provider = "CALICO"
  }

  # IP allocation policy using secondary ranges
  ip_allocation_policy {
    cluster_secondary_range_name  = var.pods_range_name
    services_secondary_range_name = var.services_range_name
  }

  # Maintenance window
  maintenance_policy {
    daily_maintenance_window {
      start_time = "03:00"
    }
  }

  # Resource labels
  resource_labels = {
    environment = var.environment
    managed-by  = "terraform"
  }

  # Remove default node pool - we'll create custom ones
  remove_default_node_pool = true
  initial_node_count       = 1

  # Master version
  min_master_version = "1.29"

  # Logging and monitoring (provider v6: component list, not `framework`)
  logging_config {
    enable_components = ["SYSTEM_COMPONENTS"]
  }
  monitoring_config {
    managed_prometheus {
      enabled = false # Disabled per design - observability is non-goal
    }
  }

  # Confidential nodes (disable for DinD compatibility)
  confidential_nodes {
    enabled = false
  }

  # Shielded nodes (provider v6: flat attribute, not a block)
  enable_shielded_nodes = true

  # Binary authorization
  binary_authorization {
    evaluation_mode = "DISABLED" # Kyverno will handle image verification
  }

  # Dataplane V2 (must be true for recent GKE versions)
  datapath_provider = "ADVANCED_DATAPATH"

  # Intra-node visibility (provider v6: flat attribute, not a block)
  enable_intranode_visibility = true

  lifecycle {
    ignore_changes = [
      # Ignore changes to node pools managed separately
      node_pool,
    ]
  }

  timeouts {
    create = "45m"
    update = "45m"
    delete = "45m"
  }
}

# Default node pool - for regular services
resource "google_container_node_pool" "default_pool" {
  name     = "default-pool"
  project  = var.project_id
  location = google_container_cluster.cluster.location
  cluster  = google_container_cluster.cluster.name

  version    = google_container_cluster.cluster.min_master_version
  node_count = var.default_node_pool_initial_node_count

  management {
    auto_repair  = true
    auto_upgrade = true
  }

  autoscaling {
    min_node_count = var.default_node_pool_min_nodes
    max_node_count = var.default_node_pool_max_nodes
  }

  node_config {
    machine_type    = var.default_node_pool_machine_type
    disk_size_gb    = var.default_node_pool_disk_size_gb
    disk_type       = var.default_node_pool_disk_type
    image_type      = "COS_CONTAINERD" # Container-Optimized OS
    service_account = google_service_account.default_pool_sa.email

    labels = {
      pool        = "default"
      environment = var.environment
      managed-by  = "terraform"
    }

    # NOTE: no taint on the default pool — every workload (gateway, identity,
    # workspace, agent, web, migrations) schedules here with no tolerations.
    # The sandbox pool below is the only tainted pool.

    # Spot VM configuration
    preemptible = var.default_node_pool_spot

    # Enable secure boot for nodes
    shielded_instance_config {
      enable_secure_boot          = true
      enable_integrity_monitoring = true
    }

    # Confidential nodes stay disabled cluster-wide (see `confidential_nodes`
    # on the cluster) — provider v6 removed this node-level argument.

    # OAuth scopes - minimal set
    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]

    # Metadata
    metadata = {
      disable-legacy-endpoints = "true"
    }
  }

  lifecycle {
    ignore_changes = [node_count]
  }

  timeouts {
    create = "30m"
    update = "30m"
    delete = "30m"
  }
}

# Sandbox node pool - for Docker-in-Docker with privileged taint
resource "google_container_node_pool" "sandbox_pool" {
  name     = "sandbox-pool"
  project  = var.project_id
  location = google_container_cluster.cluster.location
  cluster  = google_container_cluster.cluster.name

  version    = google_container_cluster.cluster.min_master_version
  node_count = var.sandbox_node_pool_initial_node_count

  management {
    auto_repair  = true
    auto_upgrade = true
  }

  autoscaling {
    min_node_count = var.sandbox_node_pool_min_nodes
    max_node_count = var.sandbox_node_pool_max_nodes
  }

  node_config {
    machine_type    = var.sandbox_node_pool_machine_type
    disk_size_gb    = var.sandbox_node_pool_disk_size_gb
    disk_type       = var.sandbox_node_pool_disk_type
    image_type      = "COS_CONTAINERD"
    service_account = google_service_account.default_pool_sa.email

    labels = {
      pool        = "sandbox"
      environment = var.environment
      managed-by  = "terraform"
      # Slash-free scheduler label for the executor's nodeSelector. (GCE
      # instance labels forbid `/`; the taint key below keeps the
      # `aaks/sandbox` form because taints are Kubernetes-only.)
      # NOTE: quoted key — HCL map keys containing `-` must be quoted.
      "aaks-sandbox" = "true"
    }

    # Taint to repel non-sandbox workloads
    taint {
      key    = "aaks/sandbox"
      value  = "true"
      effect = "NO_SCHEDULE"
    }

    # Spot VM configuration
    preemptible = var.sandbox_node_pool_spot

    # Shielded nodes
    shielded_instance_config {
      enable_secure_boot          = true
      enable_integrity_monitoring = true
    }

    # (Confidential nodes stay disabled cluster-wide; DinD needs runc.)

    # OAuth scopes
    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]

    # Metadata
    metadata = {
      disable-legacy-endpoints = "true"
    }

    # NOTE: no `sandbox { sandbox_type = "gvisor" }` here on purpose — gVisor
    # cannot run privileged pods, and this pool exists to run the privileged
    # Docker-in-Docker sidecar. Standard containerd runtime (runc) it is.
  }

  upgrade_settings {
    max_surge       = 1
    max_unavailable = 0
  }

  lifecycle {
    ignore_changes = [node_count]
  }

  timeouts {
    create = "30m"
    update = "30m"
    delete = "30m"
  }
}

# Service account for node pools (for Workload Identity)
resource "google_service_account" "default_pool_sa" {
  project      = var.project_id
  account_id   = "${var.cluster_name}-node-pool"
  display_name = "Service account for ${var.cluster_name} node pool"

  description = "Service account used by GKE nodes for accessing GCP services via Workload Identity"
}

# Minimal IAM for the node pool SA
resource "google_project_iam_member" "node_pool_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.default_pool_sa.email}"
}

resource "google_project_iam_member" "node_pool_metrics_writer" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.default_pool_sa.email}"
}

resource "google_project_iam_member" "node_pool_metrics_viewer" {
  project = var.project_id
  role    = "roles/monitoring.viewer"
  member  = "serviceAccount:${google_service_account.default_pool_sa.email}"
}
