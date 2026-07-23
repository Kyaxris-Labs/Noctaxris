output "bucket_name" {
  value = aws_s3_bucket.orders.bucket
}

output "kms_key_id" {
  value = aws_kms_key.lab.key_id
}

output "kms_key_arn" {
  value = aws_kms_key.lab.arn
}

output "table_name" {
  value = aws_dynamodb_table.orders.name
}

output "lambda_role_arn" {
  value = aws_iam_role.lambda.arn
}

output "queue_url" {
  value = aws_sqs_queue.orders.url
}

output "queue_arn" {
  value = aws_sqs_queue.orders.arn
}

output "topic_arn" {
  value = aws_sns_topic.orders.arn
}

output "function_name" {
  value = aws_lambda_function.orders.function_name
}

output "event_bus_name" {
  value = aws_cloudwatch_event_bus.lab.name
}

output "rule_name" {
  value = aws_cloudwatch_event_rule.orders.name
}

output "ssm_parameter_name" {
  value = aws_ssm_parameter.config.name
}

output "ssm_secure_parameter_name" {
  value = aws_ssm_parameter.secure_config.name
}

output "secret_arn" {
  value = aws_secretsmanager_secret.api.arn
}
