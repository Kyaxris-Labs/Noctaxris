output "launch_configuration_name" {
  value = aws_launch_configuration.lab.name
}

output "asg_name" {
  value = aws_autoscaling_group.lab.name
}

output "asg_desired_capacity" {
  value = aws_autoscaling_group.lab.desired_capacity
}

output "eks_cluster_name" {
  value = aws_eks_cluster.lab.name
}

output "eks_cluster_arn" {
  value = aws_eks_cluster.lab.arn
}

output "eks_role_arn" {
  value = aws_iam_role.eks.arn
}
