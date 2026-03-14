terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    s3  = var.cloudia_endpoint
    iam = var.cloudia_endpoint
  }
}

variable "cloudia_endpoint" {
  description = "Cloudia server endpoint"
  type        = string
  default     = "http://127.0.0.1:4566"
}

# S3 バケット
resource "aws_s3_bucket" "test" {
  bucket = "tf-e2e-test-bucket"
}

# IAM ロール
resource "aws_iam_role" "test" {
  name = "tf-e2e-test-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      }
    ]
  })
}

output "bucket_id" {
  value = aws_s3_bucket.test.id
}

output "role_name" {
  value = aws_iam_role.test.name
}
