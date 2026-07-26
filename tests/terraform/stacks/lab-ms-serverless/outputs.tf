output "bucket_name" {
  value = aws_s3_bucket.api.bucket
}

output "kms_key_arn" {
  value = aws_kms_key.lab.arn
}

output "table_name" {
  value = aws_dynamodb_table.items.name
}

output "lambda_role_arn" {
  value = aws_iam_role.lambda.arn
}

output "queue_url" {
  value = aws_sqs_queue.events.url
}

output "topic_arn" {
  value = aws_sns_topic.notify.arn
}

output "function_name" {
  value = aws_lambda_function.api.function_name
}

output "runtime" {
  value = aws_lambda_function.api.runtime
}

output "event_bus_name" {
  value = aws_cloudwatch_event_bus.lab.name
}

output "rule_name" {
  value = aws_cloudwatch_event_rule.api.name
}

output "ssm_parameter_name" {
  value = aws_ssm_parameter.config.name
}

output "secret_arn" {
  value = aws_secretsmanager_secret.api.arn
}
