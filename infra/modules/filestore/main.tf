# Filestore Module - NFS instance for git clone and worktree storage

locals {
  instance_name = "${var.instance_name}-${var.environment}"
  full_zone     = "${var.region}-${var.zone_suffix}"
}

resource "google_filestore_instance" "filestore" {
  name     = local.instance_name
  project  = var.project_id
  location = local.full_zone
  tier     = var.tier

  file_shares {
    capacity_gb = var.capacity_gb
    name        = "clone_root"
  }

  networks {
    network         = var.network_id
    modes           = ["MODE_IPV4"]
  }

  labels = {
    environment = var.environment
    managed-by  = "terraform"
  }

  timeouts {
    create = "90m"
    update = "90m"
    delete = "90m"
  }
}
