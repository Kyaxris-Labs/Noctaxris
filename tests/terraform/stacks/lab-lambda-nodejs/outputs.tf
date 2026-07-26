output "function_name" {
  value = aws_lambda_function.lab.function_name
}

output "function_arn" {
  value = aws_lambda_function.lab.arn
}

output "runtime" {
  value = aws_lambda_function.lab.runtime
}

output "lambda_role_arn" {
  value = aws_iam_role.lambda.arn
}
