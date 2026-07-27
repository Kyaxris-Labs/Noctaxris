# Auto Scaling

**Status:** shipped (lab core)

Query-protocol control-plane lab (`Action=...`, credential scope `autoscaling`). Launch configurations and auto scaling groups are stored state. `CreateAutoScalingGroup`, `UpdateAutoScalingGroup`, and `SetDesiredCapacity` reconcile lab EC2 membership to `DesiredCapacity` (clamped to min/max) via `store.RunInstances` / terminate.

Without the nested engine (`DockerHost` unset or DinD down), launched instances stay `pending` and appear on the group as `LifecycleState=Pending`. When the engine is up, pending members are started as nested containers and move to `InService` once EC2 state is `running`.

## Implemented

| Area | Actions |
|------|---------|
| Launch configurations | `CreateLaunchConfiguration`, `DescribeLaunchConfigurations`, `DeleteLaunchConfiguration` |
| Groups | `CreateAutoScalingGroup`, `DescribeAutoScalingGroups`, `UpdateAutoScalingGroup`, `DeleteAutoScalingGroup` |
| Capacity | `SetDesiredCapacity` (reconciles lab EC2 count) |
| Force delete | `DeleteAutoScalingGroup` with `ForceDelete=true` terminates attached lab EC2 instances |

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
  --min-size 1 --max-size 3 --desired-capacity 2 \
  --availability-zones us-east-1a \
  --endpoint-url "$EP"
aws autoscaling describe-auto-scaling-groups --auto-scaling-group-names asg-lab --endpoint-url "$EP"
aws autoscaling set-desired-capacity --auto-scaling-group-name asg-lab --desired-capacity 3 --endpoint-url "$EP"
aws autoscaling delete-auto-scaling-group --auto-scaling-group-name asg-lab --force-delete --endpoint-url "$EP"
```

Unit tests cover capacity clamp/scale math without Docker. Nested start soft-skips when the engine is unavailable (instances remain Pending). SDK asserts `InstanceIds` after `SetDesiredCapacity` (soft-skip when API down). Terraform: `STACK=lab-parity-compute` (`TF_PARITY=1` / `NOCTAXRIS_ADVANCED=1`).

## Not yet / deferred

- Lifecycle hooks, scaling policies, target group attach
- Mixed instances / launch templates
- `DescribeAutoScalingInstances` / attach-detach existing instances
