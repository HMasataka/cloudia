terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

variable "cloudia_endpoint" {
  description = "Cloudia server endpoint"
  type        = string
  default     = "http://127.0.0.1:4566"
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    s3          = var.cloudia_endpoint
    ec2         = var.cloudia_endpoint
    iam         = var.cloudia_endpoint
    sqs         = var.cloudia_endpoint
    dynamodb    = var.cloudia_endpoint
    rds         = var.cloudia_endpoint
    elasticache = var.cloudia_endpoint
    lambda      = var.cloudia_endpoint
    eks         = var.cloudia_endpoint
  }
}

# S3 バケット
resource "aws_s3_bucket" "example" {
  bucket = "my-example-bucket"
}

# EC2 インスタンス
resource "aws_instance" "example" {
  ami           = "ami-example"
  instance_type = "t2.micro"

  tags = {
    Name = "example-instance"
  }
}

# IAM ロール
resource "aws_iam_role" "example" {
  name = "example-role"

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

# SQS キュー
resource "aws_sqs_queue" "example" {
  name = "example-queue"
}

# DynamoDB テーブル
resource "aws_dynamodb_table" "example" {
  name         = "example-table"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"

  attribute {
    name = "id"
    type = "S"
  }
}

# RDS インスタンス
resource "aws_db_instance" "example" {
  identifier     = "example-db"
  engine         = "mysql"
  instance_class = "db.t3.micro"
  username       = "admin"
  password       = "password1234"
}

output "bucket_id" {
  value = aws_s3_bucket.example.id
}

output "instance_id" {
  value = aws_instance.example.id
}
