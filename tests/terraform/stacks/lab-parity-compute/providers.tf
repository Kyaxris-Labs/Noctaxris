# Custom endpoints for Noctaxris (see HashiCorp custom-service-endpoints guide).
# Skip IMDS. STS/IAM included because the provider probes them for EKS role create.

variable "endpoint" {
  type        = string
  description = "Noctaxris API base URL"
  default     = "http://127.0.0.1:4566"
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "access_key" {
  type    = string
  default = "AKIAROOTEXAMPLE01"
}

variable "secret_key" {
  type      = string
  sensitive = true
  default   = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
}

variable "name_prefix" {
  type        = string
  description = "Unique prefix for resource names"
}

provider "aws" {
  access_key = var.access_key
  secret_key = var.secret_key
  region     = var.region

  skip_credentials_validation = true
  skip_metadata_api_check     = true
  s3_use_path_style           = true

  endpoints {
    sts          = var.endpoint
    iam          = var.endpoint
    ec2          = var.endpoint
    autoscaling  = var.endpoint
    eks          = var.endpoint
  }
}
