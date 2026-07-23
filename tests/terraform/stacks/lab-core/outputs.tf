output "bucket_name" {
  value = aws_s3_bucket.lab.bucket
}

output "role_arn" {
  value = aws_iam_role.lab.arn
}

output "table_name" {
  value = aws_dynamodb_table.lab.name
}

output "kms_key_id" {
  value = aws_kms_key.lab.key_id
}
