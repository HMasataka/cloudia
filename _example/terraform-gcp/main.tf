terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
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

provider "google" {
  project = var.project_id
  region  = "us-central1"
  zone    = "us-central1-a"

  credentials = jsonencode({
    type           = "service_account"
    project_id     = var.project_id
    private_key_id = "test"
    private_key    = "test"
    client_email   = "test@cloudia-local.iam.gserviceaccount.com"
    client_id      = "test"
    auth_uri       = "https://accounts.google.com/o/oauth2/auth"
    token_uri      = "${var.cloudia_endpoint}/token"
  })
}

# Cloud Storage バケット
resource "google_storage_bucket" "example" {
  name     = "my-example-bucket"
  location = "US"

  lifecycle {
    prevent_destroy = false
  }
}

# Compute Engine インスタンス
resource "google_compute_instance" "example" {
  name         = "example-instance"
  machine_type = "e2-micro"
  zone         = "us-central1-a"

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-11"
    }
  }

  network_interface {
    network = "default"
  }
}

# Cloud SQL インスタンス
resource "google_sql_database_instance" "example" {
  name             = "example-db"
  database_version = "MYSQL_8_0"
  region           = "us-central1"

  settings {
    tier = "db-f1-micro"
  }
}

# Pub/Sub トピック
resource "google_pubsub_topic" "example" {
  name = "example-topic"
}

# Pub/Sub サブスクリプション
resource "google_pubsub_subscription" "example" {
  name  = "example-subscription"
  topic = google_pubsub_topic.example.id
}

output "bucket_name" {
  value = google_storage_bucket.example.name
}

output "instance_name" {
  value = google_compute_instance.example.name
}
