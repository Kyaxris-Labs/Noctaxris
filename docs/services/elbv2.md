# Elastic Load Balancing v2

**Status:** shipped (lab core)

Application load balancer, target group, and listener CRUD lite. Target types `lambda` and `ip` only. No EC2 instance targets. Identity authz.

## Implemented

| Area | Actions |
|------|---------|
| Load balancer | `CreateLoadBalancer`, `DescribeLoadBalancers`, `DeleteLoadBalancer` |
| Target group | `CreateTargetGroup`, `DescribeTargetGroups`, `DeleteTargetGroup` |
| Listener | `CreateListener`, `DescribeListeners`, `DeleteListener` |
| Targets | `RegisterTargets`, `DescribeTargetHealth` (healthy stub) |

### Authz notes

Identity `EvaluateFull` on `elasticloadbalancing:*`.

`TargetType` `instance` is rejected. Lambda targets require a function ARN. IP targets require a parseable lab IP.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws elbv2 create-load-balancer --name lab-alb --endpoint-url "$EP"
aws elbv2 create-target-group --name lab-tg --target-type lambda --endpoint-url "$EP"
```

Skip live smoke when Docker is unavailable (unit tests cover CRUD and lambda target registration).

## Not yet / deferred

- ALB Cognito authenticate action
- Path-based routing depth
- NLB beyond this ALB-shaped stub
- Real Lambda invoke from the listener data path
