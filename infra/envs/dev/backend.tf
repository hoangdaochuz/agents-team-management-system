# Terraform backend configuration for dev environment.
#
# State lives in GCS (per-env bucket, versioned) so `random_password` results
# (DB/Kafka credentials) never sit in a local plaintext tfstate and concurrent
# applies lock against each other. Create the bucket once (see infra/README.md):
#   gcloud storage buckets create gs://aaks-terraform-state-dev --project=<PROJECT_ID>
#   gcloud storage buckets update gs://aaks-terraform-state-dev --versioning
terraform {
  backend "gcs" {
    bucket = "aaks-terraform-state-dev"
    prefix = "terraform-state"
  }
}
