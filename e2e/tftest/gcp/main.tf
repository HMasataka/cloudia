terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = "us-central1"
  zone    = "us-central1-a"

  # ローカルエミュレーター用の設定
  credentials = jsonencode({
    type = "service_account"
    project_id = var.project_id
    private_key_id = "test"
    private_key = "test"
    client_email = "test@cloudia-local.iam.gserviceaccount.com"
    client_id = "test"
    auth_uri = "https://accounts.google.com/o/oauth2/auth"
    token_uri = "${var.cloudia_endpoint}/token"
  })
}

variable "cloudia_endpoint" {
  description = "Cloudia server endpoint"
  type        = string
  default     = "http://127.0.0.1:4566"
}

variable "project_id" {
  description = "GCP project ID"
  type        = string
  default     = "cloudia-local"
}

# GCS バケット
resource "google_storage_bucket" "test" {
  name     = "tf-e2e-gcs-test-bucket"
  location = "US"

  lifecycle {
    prevent_destroy = false
  }
}

output "bucket_name" {
  value = google_storage_bucket.test.name
}
