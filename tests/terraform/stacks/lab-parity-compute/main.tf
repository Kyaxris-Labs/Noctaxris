# Lab compute parity: launch configuration + ASG + EKS metadata cluster.
#
# aws_instance omitted: AWS provider waits for instance running; without DinD the lab
# leaves instances pending, so create hangs/times out. ASG DesiredCapacity=1 still
# exercises RunInstances via reconcile (members may stay Pending).
#
# Hardcoded ami-alpine (no DescribeImages data source; catalog API not implemented).
# wait_for_capacity_timeout=0 keeps apply green without nested engine.
# force_delete so destroy succeeds with pending members.
# NLB (aws_lb type=network) omitted: ELBv2 is lab-JSON / SDK-only (see tests/HANDOFF.md).

locals {
  ami_id = "ami-alpine"
}

resource "aws_launch_configuration" "lab" {
  name          = "${var.name_prefix}-lc"
  image_id      = local.ami_id
  instance_type = "t3.micro"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_autoscaling_group" "lab" {
  name                 = "${var.name_prefix}-asg"
  launch_configuration = aws_launch_configuration.lab.name
  desired_capacity     = 1
  min_size             = 0
  max_size             = 2
  availability_zones   = ["${var.region}a"]

  # Do not wait for InService (Pending without DinD is expected).
  wait_for_capacity_timeout = "0"
  force_delete              = true

  tag {
    key                 = "Name"
    value               = "${var.name_prefix}-asg"
    propagate_at_launch = true
  }
}

resource "aws_iam_role" "eks" {
  name = "${var.name_prefix}-eks"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "eks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_eks_cluster" "lab" {
  name     = "${var.name_prefix}-eks"
  role_arn = aws_iam_role.eks.arn
  version  = "1.29"

  # Lab accepts stub subnet IDs; no VPC plane required.
  vpc_config {
    subnet_ids              = ["subnet-lab-a", "subnet-lab-b"]
    endpoint_public_access  = true
    endpoint_private_access = false
  }

  # Avoid CreateAddon / managed-addon calls against the lab stub.
  bootstrap_self_managed_addons = false

  depends_on = [aws_iam_role.eks]
}
