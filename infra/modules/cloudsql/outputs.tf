# Cloud SQL module outputs

output "instance_name" {
  description = "Cloud SQL instance name"
  value       = google_sql_database_instance.instance.name
}

output "instance_connection_name" {
  description = "Cloud SQL connection string"
  value       = google_sql_database_instance.instance.connection_name
}

output "instance_private_ip" {
  description = "Cloud SQL private IP address"
  value       = google_sql_database_instance.instance.private_ip_address
}

output "database_names" {
  description = "List of created database names"
  value       = values(google_sql_database.databases)[*].name
}

output "db_user_secret_name" {
  description = "Secret Manager secret name for DB password"
  value       = google_secret_manager_secret.db_password.secret_id
}

output "db_user_secret_id" {
  description = "Secret Manager secret resource ID"
  value       = google_secret_manager_secret.db_password.id
}

output "db_username" {
  description = "Database username"
  value       = google_sql_user.aaks_user.name
}
