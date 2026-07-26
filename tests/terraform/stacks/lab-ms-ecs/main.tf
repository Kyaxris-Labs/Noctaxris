# Lab microservice ECS stack: ECR + task definition + service.
# live=false (default): DesiredCount 0, control-plane only (no DinD).
# live=true: DesiredCount >= 1; needs healthy noctaxris-engine. Default image alpine:3.20
#   (DinD pull). Optional: push to lab ECR via scripts/push-lab-image.sh and pass
#   -var=container_image=127.0.0.1:4566/ACCOUNT/REPO:lab
# Apply: STACK=lab-ms-ecs bash tests/terraform/run.sh
# Live:  STACK=lab-ms-ecs TF_MS_LIVE=1 bash tests/terraform/run.sh

data "aws_caller_identity" "current" {}

locals {
  desired_count = var.live ? var.desired_count : 0
  ecr_image     = "127.0.0.1:4566/${data.aws_caller_identity.current.account_id}/${aws_ecr_repository.app.name}:lab"
  # Empty container_image uses alpine for live-ready runs; ECR URI when use_ecr_image=true.
  container_image = var.container_image != "" ? var.container_image : (
    var.use_ecr_image ? local.ecr_image : "alpine:3.20"
  )
}

resource "aws_ecr_repository" "app" {
  name                 = "${var.name_prefix}-ms-app"
  image_tag_mutability = "MUTABLE"
}

resource "aws_iam_role" "task" {
  name = "${var.name_prefix}-ms-ecs-task"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role" "execution" {
  name = "${var.name_prefix}-ms-ecs-exec"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy" "execution_ecr" {
  name = "${var.name_prefix}-ms-ecs-exec-ecr"
  role = aws_iam_role.execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "ecr:BatchGetImage",
        "ecr:GetDownloadUrlForLayer",
        "ecr:GetAuthorizationToken",
      ]
      Resource = "*"
    }]
  })
}

resource "aws_ecs_task_definition" "app" {
  family                   = "${var.name_prefix}-ms-app"
  task_role_arn            = aws_iam_role.task.arn
  execution_role_arn       = aws_iam_role.execution.arn
  network_mode             = "bridge"
  requires_compatibilities = ["EC2"]
  cpu                      = "256"
  memory                   = "512"

  container_definitions = jsonencode([{
    name      = "app"
    image     = local.container_image
    essential = true
    command   = ["echo", "lab-ms-ecs-ok"]
  }])
}

resource "aws_ecs_service" "app" {
  name            = "${var.name_prefix}-ms-svc"
  cluster         = "default"
  task_definition = aws_ecs_task_definition.app.arn
  desired_count   = local.desired_count
  # No awsvpc / load_balancer: lab ECS rejects awsvpcConfiguration.
}
