output "repository_url" {
  value = aws_ecr_repository.app.repository_url
}

output "repository_name" {
  value = aws_ecr_repository.app.name
}

output "task_definition_arn" {
  value = aws_ecs_task_definition.app.arn
}

output "service_name" {
  value = aws_ecs_service.app.name
}

output "desired_count" {
  value = aws_ecs_service.app.desired_count
}

output "live" {
  value = var.live
}

output "container_image" {
  value = local.container_image
}

output "task_role_arn" {
  value = aws_iam_role.task.arn
}

output "execution_role_arn" {
  value = aws_iam_role.execution.arn
}
