# lab-parity-compute

Terraform parity stack for Auto Scaling + EKS against Noctaxris (EC2 via ASG reconcile).

| Resource | Included | Notes |
|----------|----------|-------|
| `aws_instance` | No | Provider waits for `running`; without DinD lab stays `pending` and create hangs. ASG covers RunInstances. |
| `aws_launch_configuration` + `aws_autoscaling_group` | Yes | DesiredCapacity 1, Min 0, Max 2; hardcoded `ami-alpine` (DescribeImages used by provider for LC); `wait_for_capacity_timeout=0`; `force_delete` |
| `aws_eks_cluster` | Yes | Metadata ACTIVE; IAM role + stub subnet IDs; `bootstrap_self_managed_addons=false` |
| `aws_lb` (NLB) | No | ELBv2 lab-JSON is SDK-only |

Endpoints: `ec2`, `autoscaling`, `eks`, `iam`, `sts`.

```bash
STACK=lab-parity-compute bash tests/terraform/run.sh
# or via suite (uses docker/.env root keys via your shell env):
TF_PARITY=1 bash tests/run-all.sh
```
