# Lab Lambda Java zip runtimes (java21 + java25).
# Apply/destroy: STACK=lab-lambda-java bash tests/terraform/run.sh
# Zip holds source under example/; CreateFunction CRUD does not require a compiled JAR.

data "archive_file" "lambda_zip" {
  type        = "zip"
  source_dir  = "${path.module}/lambda"
  output_path = "${path.module}/.build/lambda.zip"
}

resource "aws_iam_role" "lambda" {
  name = "${var.name_prefix}-lambda-java"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_lambda_function" "java21" {
  function_name    = "${var.name_prefix}-java21"
  role             = aws_iam_role.lambda.arn
  handler          = "example.Handler::handleRequest"
  runtime          = "java21"
  filename         = data.archive_file.lambda_zip.output_path
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
}

resource "aws_lambda_function" "java25" {
  function_name    = "${var.name_prefix}-java25"
  role             = aws_iam_role.lambda.arn
  handler          = "example.Handler::handleRequest"
  runtime          = "java25"
  filename         = data.archive_file.lambda_zip.output_path
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
}
