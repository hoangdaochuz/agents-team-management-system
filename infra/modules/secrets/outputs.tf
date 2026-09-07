# Secrets module outputs

output "agent_master_key_secret_name" {
  description = "Secret name for agent master key (provider-key encryption)"
  value       = google_secret_manager_secret.agent_master_key.secret_id
}

output "agent_master_key_secret_id" {
  description = "Secret resource ID for agent master key"
  value       = google_secret_manager_secret.agent_master_key.id
}

output "internal_token_secret_name" {
  description = "Secret name for the shared internal service token"
  value       = google_secret_manager_secret.internal_token.secret_id
}

output "internal_token_secret_id" {
  description = "Secret resource ID for the shared internal token"
  value       = google_secret_manager_secret.internal_token.id
}

output "agent_internal_token_secret_name" {
  description = "Secret name for the agent service internal token"
  value       = google_secret_manager_secret.agent_internal_token.secret_id
}

output "agent_internal_token_secret_id" {
  description = "Secret resource ID for the agent internal token"
  value       = google_secret_manager_secret.agent_internal_token.id
}

output "db_dsn_secret_names" {
  description = "Secret names for the per-service database DSNs"
  value       = { for k, v in google_secret_manager_secret.db_dsns : k => v.secret_id }
}

output "auth_seed_superadmin_email_secret_name" {
  description = "Secret name for seed superadmin email"
  value       = google_secret_manager_secret.auth_seed_superadmin_email.secret_id
}

output "auth_seed_superadmin_email_secret_id" {
  description = "Secret resource ID for seed superadmin email"
  value       = google_secret_manager_secret.auth_seed_superadmin_email.id
}

output "auth_seed_superadmin_password_secret_name" {
  description = "Secret name for seed superadmin password"
  value       = google_secret_manager_secret.auth_seed_superadmin_password.secret_id
}

output "auth_seed_superadmin_password_secret_id" {
  description = "Secret resource ID for seed superadmin password"
  value       = google_secret_manager_secret.auth_seed_superadmin_password.id
}
