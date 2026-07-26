# Lab Lambda Node.js zip runtime (nodejs24.x).
# Apply/destroy: STACK=lab-lambda-nodejs bash tests/terraform/run.sh

data "archive_file" "lambda_zip" {
  type        = "zip"
  source_file = "${path.module}/lambda/index.js"
  output_path = "${path.module}/.build/lambda.zip"
}

resource "aws_iam_role" "lambda" {
  name = "${var.name_prefix}-lambda-node"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_lambda_function" "lab" {
  function_name    = "${var.name_prefix}-node24"
  role             = aws_iam_role.lambda.arn
  handler          = "index.handler"
  runtime          = "nodejs24.x"
  filename         = data.archive_file.lambda_zip.output_path
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
}
