# Custom endpoints for Noctaxris (see HashiCorp custom-service-endpoints guide).
# Path-style S3 is required. Skip IMDS and account-id lookup.
# Credentials are AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY in the environment
# (the standalone account key).

variable "endpoint" {
  type        = string
  description = "Noctaxris API base URL (NOCTAXRIS_ENDPOINT)"
  default     = "http://127.0.0.1:4566"
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "bucket_name" {
  type        = string
  description = "S3 bucket name. The resource applies lower()."
  default     = "standalone-lab-bucket"
}

provider "aws" {
  region = var.region

  skip_credentials_validation = true
  skip_requesting_account_id  = true
  skip_metadata_api_check     = true
  s3_use_path_style           = true

  endpoints {
    sts      = var.endpoint
    iam      = var.endpoint
    s3       = var.endpoint
    dynamodb = var.endpoint
    kms      = var.endpoint
  }
}
