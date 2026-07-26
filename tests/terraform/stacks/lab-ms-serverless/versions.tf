terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      # Match lab-fullstack (~> 5.x). Provider 6.x calls S3 Control ListTagsForResource
      # which Noctaxris does not implement.
      version = "~> 5.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.0"
    }
  }
}
