# Advanced lab-fullstack stack: multi-resource order-pipeline shape.
# Apply/destroy via: STACK=lab-fullstack bash tests/terraform/run.sh
# Not wired into tests/run-all.sh until stable green.

data "archive_file" "lambda_zip" {
  type        = "zip"
  source_file = "${path.module}/lambda/index.py"
  output_path = "${path.module}/.build/lambda.zip"
}

resource "aws_kms_key" "lab" {
  description             = "${var.name_prefix} lab cmk"
  deletion_window_in_days = 7
}

resource "aws_iam_role" "lambda" {
  name = "${var.name_prefix}-lambda"

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
  name = "${var.name_prefix}-lambda-inline"
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

resource "aws_s3_bucket" "orders" {
  bucket        = "${var.name_prefix}-orders"
  force_destroy = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "orders" {
  bucket = aws_s3_bucket.orders.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.lab.arn
    }
  }
}

resource "aws_dynamodb_table" "orders" {
  name         = "${var.name_prefix}-orders"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "orderId"

  attribute {
    name = "orderId"
    type = "S"
  }

  server_side_encryption {
    enabled     = true
    kms_key_arn = aws_kms_key.lab.arn
  }
}

resource "aws_sqs_queue" "orders" {
  name = "${var.name_prefix}-orders"
}

resource "aws_sqs_queue_policy" "orders_events" {
  queue_url = aws_sqs_queue.orders.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "AllowEventBridgeSend"
      Effect    = "Allow"
      Principal = { Service = "events.amazonaws.com" }
      Action    = "sqs:SendMessage"
      Resource  = aws_sqs_queue.orders.arn
      Condition = {
        ArnEquals = {
          "aws:SourceArn" = aws_cloudwatch_event_rule.orders.arn
        }
      }
    }]
  })
}

resource "aws_sns_topic" "orders" {
  name = "${var.name_prefix}-orders"
}

resource "aws_lambda_function" "orders" {
  function_name    = "${var.name_prefix}-orders"
  role             = aws_iam_role.lambda.arn
  handler          = "index.handler"
  runtime          = "python3.12"
  filename         = data.archive_file.lambda_zip.output_path
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256

  # Invoke needs nested DinD; CRUD/apply is enough for this stack.
}

resource "aws_cloudwatch_event_bus" "lab" {
  name = "${var.name_prefix}-bus"
}

resource "aws_cloudwatch_event_rule" "orders" {
  name           = "${var.name_prefix}-orders"
  event_bus_name = aws_cloudwatch_event_bus.lab.name
  event_pattern = jsonencode({
    source = ["noctaxris.lab.orders"]
  })
}

resource "aws_cloudwatch_event_target" "orders_sqs" {
  rule           = aws_cloudwatch_event_rule.orders.name
  event_bus_name = aws_cloudwatch_event_bus.lab.name
  target_id      = "orders-sqs"
  arn            = aws_sqs_queue.orders.arn
}

resource "aws_cloudwatch_event_target" "orders_sns" {
  rule           = aws_cloudwatch_event_rule.orders.name
  event_bus_name = aws_cloudwatch_event_bus.lab.name
  target_id      = "orders-sns"
  arn            = aws_sns_topic.orders.arn
}

resource "aws_ssm_parameter" "config" {
  name  = "/lab/${var.name_prefix}/config"
  type  = "String"
  value = "lab-fullstack"
}

resource "aws_ssm_parameter" "stringlist" {
  name  = "/lab/${var.name_prefix}/stringlist"
  type  = "StringList"
  value = "one,two,three"
}

resource "aws_ssm_parameter" "secure_config" {
  name   = "/lab/${var.name_prefix}/secure-config"
  type   = "SecureString"
  value  = "lab-fullstack-secret"
  key_id = aws_kms_key.lab.id
}

resource "aws_secretsmanager_secret" "api" {
  name       = "${var.name_prefix}-api"
  kms_key_id = aws_kms_key.lab.id
}

resource "aws_secretsmanager_secret_version" "api" {
  secret_id     = aws_secretsmanager_secret.api.id
  secret_string = jsonencode({ token = "lab-fullstack" })
}
