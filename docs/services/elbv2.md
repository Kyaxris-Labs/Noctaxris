# Elastic Load Balancing v2

**Status:** shipped (lab core)

Application load balancer, target group, and listener CRUD lite plus an **ELB lab listener** HTTP path that invokes Lambda targets. Target types `lambda` and `ip` only. No EC2 instance targets. Identity authz on control-plane APIs. Compose publishes only `127.0.0.1:4566` (no open internet).

## Implemented

| Area | Actions |
|------|---------|
| Load balancer | `CreateLoadBalancer`, `DescribeLoadBalancers`, `DeleteLoadBalancer` (`Type=application` only; `Type=network` rejected) |
| Target group | `CreateTargetGroup`, `DescribeTargetGroups`, `DeleteTargetGroup` |
| Listener | `CreateListener`, `DescribeListeners`, `DeleteListener` (forward to target group) |
| Targets | `RegisterTargets`, `DescribeTargetHealth` (`healthy` when a listener forwards and Lambda permission Allows; `unused` without a listener; IP stays `unused`) |
| Lab listener | `GET`/`POST` `/alb/{accountId}/{loadBalancerName}/{port}[/{path...}]` invokes the first registered Lambda (ALB event shape) |

### Authz notes

Identity `EvaluateFull` on `elasticloadbalancing:*`.

`TargetType` `instance` is rejected. Lambda targets must resolve to an existing function and Allow `elasticloadbalancing.amazonaws.com` on the function resource policy (`lambda:AddPermission`; optional `SourceArn` = target group ARN). IP targets require a parseable lab IP (no IP dataplane).

The lab listener is an open dataplane path gated like Function URL `NONE`: allowed on loopback listen by default; non-loopback requires `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1`. Associated WAFv2 Web ACLs on the load balancer ARN are enforced on invoke.

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
aws elbv2 create-listener \
  --load-balancer-arn "$LB_ARN" \
  --protocol HTTP --port 80 \
  --default-actions Type=forward,TargetGroupArn="$TG_ARN" \
  --endpoint-url "$EP"
aws elbv2 describe-target-health --target-group-arn "$TG_ARN" --endpoint-url "$EP"
curl -sS -X POST "http://127.0.0.1:4566/alb/$ACCOUNT/lab-alb/80/hello" -d '{"ok":true}'
```

Expect `State=healthy` on DescribeTargetHealth after CreateListener + RegisterTargets with permission. Live `curl` needs healthy DinD (`noctaxris-engine`); unit tests cover the path with an invoke hook. Skip live smoke when Docker is unavailable.

## Not yet / deferred

- ALB Cognito authenticate action
- Path-based routing depth (rules beyond default forward)
- NLB (`Type=network`) ARN path
- IP / instance target dataplane
- CloudFront live PoP / fake-edge
