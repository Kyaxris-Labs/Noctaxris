terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      # python3.14 runtime enum landed in provider 6.21.0 (Context7 / provider changelog).
      version = ">= 6.21.0, < 7.0.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.0"
    }
  }
}
