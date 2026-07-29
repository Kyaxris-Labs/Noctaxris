# Auto Scaling

**Status:** shipped (lab core)

Query-protocol control-plane lab (`Action=...`, credential scope `autoscaling`). Launch configurations and auto scaling groups are stored state. `CreateAutoScalingGroup`, `UpdateAutoScalingGroup`, and `SetDesiredCapacity` reconcile lab EC2 membership to `DesiredCapacity` (clamped to min/max) via `store.RunInstances` / terminate.

Without the nested engine (`DockerHost` unset or DinD down), launched instances stay `pending` and appear on the group as `LifecycleState=Pending`. When the engine is up, pending members are started as nested containers and move to `InService` once EC2 state is `running`.

Scaling policies, lifecycle hooks, and target-group attachments are stored state. `AttachInstances` / `DetachInstances` update ASG membership and DesiredCapacity without requiring DinD. `AttachLoadBalancerTargetGroups` validates each ARN against the ELBv2 target-group store.

## Implemented

| Area | Actions |
|------|---------|
| Launch configurations | `CreateLaunchConfiguration`, `DescribeLaunchConfigurations`, `DeleteLaunchConfiguration` |
| Groups | `CreateAutoScalingGroup`, `DescribeAutoScalingGroups`, `UpdateAutoScalingGroup`, `DeleteAutoScalingGroup` |
| Capacity | `SetDesiredCapacity` (reconciles lab EC2 count) |
| Force delete | `DeleteAutoScalingGroup` with `ForceDelete=true` terminates attached lab EC2 instances |
| Scaling policies | `PutScalingPolicy`, `DescribePolicies`, `DeletePolicy` (SimpleScaling and TargetTrackingScaling fields stored) |
| Lifecycle hooks | `PutLifecycleHook`, `DescribeLifecycleHooks`, `DeleteLifecycleHook` |
| Instances | `AttachInstances`, `DetachInstances`, `DescribeAutoScalingInstances` (membership without DinD) |
| Target groups | `AttachLoadBalancerTargetGroups`, `DetachLoadBalancerTargetGroups`, `DescribeLoadBalancerTargetGroups` (ELBv2 ARN validated when present) |

### Authz notes

Identity `EvaluateFull` on `autoscaling:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws autoscaling create-launch-configuration \
  --launch-configuration-name lc-lab \
  --image-id ami-alpine \
  --instance-type t3.micro \
  --endpoint-url "$EP"
aws autoscaling create-auto-scaling-group \
  --auto-scaling-group-name asg-lab \
  --launch-configuration-name lc-lab \
  --min-size 0 --max-size 4 --desired-capacity 0 \
  --availability-zones us-east-1a \
  --endpoint-url "$EP"
aws autoscaling put-scaling-policy \
  --auto-scaling-group-name asg-lab \
  --policy-name scale-out \
  --policy-type SimpleScaling \
  --adjustment-type ChangeInCapacity \
  --scaling-adjustment 1 \
  --endpoint-url "$EP"
aws autoscaling put-lifecycle-hook \
  --auto-scaling-group-name asg-lab \
  --lifecycle-hook-name launch-hook \
  --lifecycle-transition autoscaling:EC2_INSTANCE_LAUNCHING \
  --default-result CONTINUE \
  --heartbeat-timeout 300 \
  --endpoint-url "$EP"
aws elbv2 create-target-group \
  --name asg-lab-tg --protocol HTTP --port 80 --target-type instance \
  --endpoint-url "$EP"
# use TargetGroupArn from create-target-group
aws autoscaling attach-load-balancer-target-groups \
  --auto-scaling-group-name asg-lab \
  --target-group-arns "$TG_ARN" \
  --endpoint-url "$EP"
aws autoscaling describe-auto-scaling-groups --auto-scaling-group-names asg-lab --endpoint-url "$EP"
aws autoscaling delete-auto-scaling-group --auto-scaling-group-name asg-lab --force-delete --endpoint-url "$EP"
```

Unit tests cover capacity clamp/scale math, scaling policies, lifecycle hooks, attach/detach without Docker, and TG ARN validation. Nested start soft-skips when the engine is unavailable (instances remain Pending). SDK asserts `InstanceIds` after `SetDesiredCapacity` (soft-skip when API down). Terraform: `STACK=lab-parity-compute` (`TF_PARITY=1` / `NOCTAXRIS_ADVANCED=1`).

## Not yet / deferred

- Mixed instances / launch templates
- Classic ELB attach; CompleteLifecycleAction / RecordLifecycleActionHeartbeat enforcement
- Automatic target registration of InService members into attached ELBv2 groups
