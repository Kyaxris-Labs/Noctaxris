resource "aws_s3_bucket" "lab" {
  bucket        = "${var.name_prefix}-bucket"
  force_destroy = true
}

resource "aws_iam_role" "lab" {
  name = "${var.name_prefix}-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_dynamodb_table" "lab" {
  name         = "${var.name_prefix}-table"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "pk"

  attribute {
    name = "pk"
    type = "S"
  }
}

resource "aws_kms_key" "lab" {
  description             = "Noctaxris lab key"
  deletion_window_in_days = 7
}
