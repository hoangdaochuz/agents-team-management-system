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

  # Private cluster configuration: private endpoint AND private nodes (nodes
  # must not get public IPs — see infra/README.md). No global master access;
  # operators reach the endpoint via authorized networks (dev) or IAP/bastion.
  private_cluster_config {
    enable_private_endpoint = true
    enable_private_nodes    = true
    master_ipv4_cidr_block  = var.master_ipv4_cidr_block

    master_global_access_config {
      enabled = false
    }
  }

  # Master authorized networks (provider v6: top-level block, NOT nested in
  # private_cluster_config). Default EMPTY (private-only): the operator reaches
  # the endpoint via IAP/bastion/VPN, or passes their CIDR via
  # `master_authorized_cidrs` (e.g. a Cloud Shell / office range for dev).
  master_authorized_networks_config {
    dynamic "cidr_blocks" {
      for_each = var.master_authorized_cidrs
      content {
        cidr_block   = cidr_blocks.value
        display_name = "operator-${cidr_blocks.key}"
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

  # Master version: unset — the REGULAR release channel + auto-upgrade own it.
  # A stale `min_master_version` pin fights the channel (GKE rejects EOL minors).

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

  # Node version: unset — tracks the channel-owned master version.
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

    # Spot VM configuration (provider v6: `spot`, not deprecated `preemptible`)
    spot = var.default_node_pool_spot

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

  upgrade_settings {
    max_surge       = 1
    max_unavailable = 0
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

  # Node version: unset — tracks the channel-owned master version.
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
    service_account = google_service_account.sandbox_pool_sa.email

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

    # Spot VM configuration (provider v6: `spot`, not deprecated `preemptible`)
    spot = var.sandbox_node_pool_spot

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

# Separate SA for the sandbox pool: a privileged-DinD node compromise must
# not inherit even the default pool's (minimal) identity.
resource "google_service_account" "sandbox_pool_sa" {
  project      = var.project_id
  account_id   = "${var.cluster_name}-sandbox-pool"
  display_name = "Service account for ${var.cluster_name} sandbox node pool"

  description = "Service account used by sandbox GKE nodes (DinD plane)"
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

# Same minimal trio for the sandbox SA (kept separate — see above).
resource "google_project_iam_member" "sandbox_pool_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.sandbox_pool_sa.email}"
}

resource "google_project_iam_member" "sandbox_pool_metrics_writer" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.sandbox_pool_sa.email}"
}

resource "google_project_iam_member" "sandbox_pool_metrics_viewer" {
  project = var.project_id
  role    = "roles/monitoring.viewer"
  member  = "serviceAccount:${google_service_account.sandbox_pool_sa.email}"
}
