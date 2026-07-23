# Elastic Load Balancing v2

**Status:** shipped (lab core, control-plane stub)

Application load balancer, target group, and listener CRUD lite. Target types `lambda` and `ip` only. No EC2 instance targets. Identity authz. No lab listener dataplane (targets stay `unused`).

## Implemented

| Area | Actions |
|------|---------|
| Load balancer | `CreateLoadBalancer`, `DescribeLoadBalancers`, `DeleteLoadBalancer` (`Type=application` only; `Type=network` rejected) |
| Target group | `CreateTargetGroup`, `DescribeTargetGroups`, `DeleteTargetGroup` |
| Listener | `CreateListener`, `DescribeListeners`, `DeleteListener` (stored forward only) |
| Targets | `RegisterTargets`, `DescribeTargetHealth` (`unused` until a lab listener exists) |

### Authz notes

Identity `EvaluateFull` on `elasticloadbalancing:*`.

`TargetType` `instance` is rejected. Lambda targets must resolve to an existing function and Allow `elasticloadbalancing.amazonaws.com` on the function resource policy (`lambda:AddPermission`; optional `SourceArn` = target group ARN). IP targets require a parseable lab IP. Health is never `healthy` without a listener invoke path.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws elbv2 create-load-balancer --name lab-alb --endpoint-url "$EP"
aws elbv2 create-target-group --name lab-tg --target-type lambda --endpoint-url "$EP"
aws lambda add-permission \
  --function-name lab-fn \
  --statement-id elb-invoke \
  --action lambda:InvokeFunction \
  --principal elasticloadbalancing.amazonaws.com \
  --source-arn "$TG_ARN" \
  --endpoint-url "$EP"
aws elbv2 register-targets \
  --target-group-arn "$TG_ARN" \
  --targets Id="$FN_ARN" \
  --endpoint-url "$EP"
aws elbv2 describe-target-health --target-group-arn "$TG_ARN" --endpoint-url "$EP"
```

Expect `State=unused` on DescribeTargetHealth. Skip live smoke when Docker is unavailable (unit tests cover CRUD, Type reject, Lambda resolve + permission, and unused health).

## Not yet / deferred

- ALB Cognito authenticate action
- Path-based routing depth
- NLB (`Type=network`) ARN path
- Lab listener HTTP path that invokes Lambda (ELB lab listener)
