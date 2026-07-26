output "function_name_java21" {
  value = aws_lambda_function.java21.function_name
}

output "function_name_java25" {
  value = aws_lambda_function.java25.function_name
}

output "runtime_java21" {
  value = aws_lambda_function.java21.runtime
}

output "runtime_java25" {
  value = aws_lambda_function.java25.runtime
}

output "lambda_role_arn" {
  value = aws_iam_role.lambda.arn
}
