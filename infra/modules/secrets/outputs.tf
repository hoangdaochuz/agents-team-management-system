# Secrets module outputs

output "settings_master_key_secret_name" {
  description = "Secret name for settings master key"
  value       = google_secret_manager_secret.settings_master_key.secret_id
}

output "settings_master_key_secret_id" {
  description = "Secret resource ID for settings master key"
  value       = google_secret_manager_secret.settings_master_key.id
}

output "settings_internal_token_secret_name" {
  description = "Secret name for internal service token"
  value       = google_secret_manager_secret.settings_internal_token.secret_id
}

output "settings_internal_token_secret_id" {
  description = "Secret resource ID for internal token"
  value       = google_secret_manager_secret.settings_internal_token.id
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
