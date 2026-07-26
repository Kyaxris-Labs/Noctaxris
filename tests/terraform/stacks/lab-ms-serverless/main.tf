# Lab microservice serverless stack (Lambda API + data plane).
# Apply/destroy: STACK=lab-ms-serverless bash tests/terraform/run.sh

data "archive_file" "lambda_zip" {
  type        = "zip"
  source_file = "${path.module}/lambda/index.py"
  output_path = "${path.module}/.build/lambda.zip"
}

resource "aws_kms_key" "lab" {
  description             = "${var.name_prefix} ms serverless cmk"
  deletion_window_in_days = 7
}

resource "aws_iam_role" "lambda" {
  name = "${var.name_prefix}-ms-lambda"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy" "lambda" {
  name = "${var.name_prefix}-ms-lambda-inline"
  role = aws_iam_role.lambda.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "dynamodb:GetItem",
          "dynamodb:PutItem",
          "sqs:ReceiveMessage",
          "sqs:DeleteMessage",
          "sqs:GetQueueAttributes",
          "sns:Publish",
          "ssm:GetParameter",
          "ssm:GetParameters",
          "secretsmanager:GetSecretValue",
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents",
        ]
        Resource = "*"
      },
      {
        Effect = "Allow"
        Action = [
          "kms:Encrypt",
          "kms:Decrypt",
          "kms:GenerateDataKey",
          "kms:DescribeKey",
        ]
        Resource = aws_kms_key.lab.arn
      },
    ]
  })
}

resource "aws_s3_bucket" "api" {
  bucket        = "${var.name_prefix}-ms-api"
  force_destroy = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "api" {
  bucket = aws_s3_bucket.api.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.lab.arn
    }
  }
}

resource "aws_dynamodb_table" "items" {
  name         = "${var.name_prefix}-ms-items"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "itemId"

  attribute {
    name = "itemId"
    type = "S"
  }

  server_side_encryption {
    enabled     = true
    kms_key_arn = aws_kms_key.lab.arn
  }
}

resource "aws_sqs_queue" "events" {
  name = "${var.name_prefix}-ms-events"
}

resource "aws_sqs_queue_policy" "events" {
  queue_url = aws_sqs_queue.events.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "AllowEventBridgeSend"
      Effect    = "Allow"
      Principal = { Service = "events.amazonaws.com" }
      Action    = "sqs:SendMessage"
      Resource  = aws_sqs_queue.events.arn
      Condition = {
        ArnEquals = {
          "aws:SourceArn" = aws_cloudwatch_event_rule.api.arn
        }
      }
    }]
  })
}

resource "aws_sns_topic" "notify" {
  name = "${var.name_prefix}-ms-notify"
}

resource "aws_lambda_function" "api" {
  function_name    = "${var.name_prefix}-ms-api"
  role             = aws_iam_role.lambda.arn
  handler          = "index.handler"
  runtime          = "python3.12"
  filename         = data.archive_file.lambda_zip.output_path
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
}

resource "aws_cloudwatch_event_bus" "lab" {
  name = "${var.name_prefix}-ms-bus"
}

resource "aws_cloudwatch_event_rule" "api" {
  name           = "${var.name_prefix}-ms-api"
  event_bus_name = aws_cloudwatch_event_bus.lab.name
  event_pattern = jsonencode({
    source = ["noctaxris.lab.ms"]
  })
}

resource "aws_cloudwatch_event_target" "sqs" {
  rule           = aws_cloudwatch_event_rule.api.name
  event_bus_name = aws_cloudwatch_event_bus.lab.name
  target_id      = "ms-sqs"
  arn            = aws_sqs_queue.events.arn
}

resource "aws_cloudwatch_event_target" "sns" {
  rule           = aws_cloudwatch_event_rule.api.name
  event_bus_name = aws_cloudwatch_event_bus.lab.name
  target_id      = "ms-sns"
  arn            = aws_sns_topic.notify.arn
}

resource "aws_ssm_parameter" "config" {
  name  = "/lab/${var.name_prefix}/ms/config"
  type  = "String"
  value = "lab-ms-serverless"
}

resource "aws_ssm_parameter" "stringlist" {
  name  = "/lab/${var.name_prefix}/ms/stringlist"
  type  = "StringList"
  value = "api,worker,notify"
}

resource "aws_ssm_parameter" "secure_config" {
  name   = "/lab/${var.name_prefix}/ms/secure-config"
  type   = "SecureString"
  value  = "lab-ms-serverless-secret"
  key_id = aws_kms_key.lab.id
}

resource "aws_secretsmanager_secret" "api" {
  name       = "${var.name_prefix}-ms-api"
  kms_key_id = aws_kms_key.lab.id
}

resource "aws_secretsmanager_secret_version" "api" {
  secret_id     = aws_secretsmanager_secret.api.id
  secret_string = jsonencode({ token = "lab-ms-serverless" })
}
