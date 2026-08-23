# Filestore module outputs

output "instance_name" {
  description = "Filestore instance name"
  value       = google_filestore_instance.filestore.name
}

output "instance_id" {
  description = "Filestore instance ID"
  value       = google_filestore_instance.filestore.id
}

output "filestore_ip" {
  description = "Filestore IP address for NFS mount"
  value       = google_filestore_instance.filestore.networks[0].ip_addresses[0]
}

output "fileshare_name" {
  description = "Filestore share name"
  value       = google_filestore_instance.filestore.file_shares[0].name
}

output "mount_target" {
  description = "NFS mount target (IP:/sharename)"
  value       = "${google_filestore_instance.filestore.networks[0].ip_addresses[0]}:/${google_filestore_instance.filestore.file_shares[0].name}"
}

output "location" {
  description = "Filestore location (zone)"
  value       = google_filestore_instance.filestore.location
}
