# Terraform backend configuration for prod environment.
#
# State lives in GCS (per-env bucket, versioned) so `random_password` results
# (DB/Kafka credentials) never sit in a local plaintext tfstate and concurrent
# applies lock against each other. Create the bucket once (see infra/README.md):
#   gcloud storage buckets create gs://aaks-terraform-state-prod --project=<PROJECT_ID>
#   gcloud storage buckets update gs://aaks-terraform-state-prod --versioning
terraform {
  backend "gcs" {
    bucket = "aaks-terraform-state-prod"
    prefix = "terraform-state"
  }
}
