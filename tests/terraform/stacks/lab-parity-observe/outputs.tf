output "table_name" {
  value = aws_dynamodb_table.lab.name
}

output "lsi_name" {
  value = one(aws_dynamodb_table.lab.local_secondary_index).name
}
