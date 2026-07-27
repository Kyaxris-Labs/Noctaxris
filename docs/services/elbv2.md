# Elastic Load Balancing v2

**Status:** shipped (lab core)

Application and Network load balancer, target group, listener, and (ALB) path/host listener rule CRUD lite plus an **ELB lab listener** HTTP path that invokes Lambda targets on application LBs. Target types `lambda`, `ip`, and `instance` (lab-opaque `i-*` ids; no EC2 resolve). Identity authz on control-plane APIs. Compose publishes only `127.0.0.1:4566` (no open internet).

## Implemented

| Area | Actions |
|------|---------|
| Load balancer | `CreateLoadBalancer`, `DescribeLoadBalancers`, `DeleteLoadBalancer` (`Type=application` or `Type=network`; other values rejected). ARNs use `loadbalancer/app/...` or `loadbalancer/net/...` |
| Target group | `CreateTargetGroup`, `DescribeTargetGroups`, `DeleteTargetGroup` (`lambda` / `ip` / `instance`; NLB-oriented groups use `Protocol=TCP` or `TLS`) |
| Listener | `CreateListener`, `DescribeListeners`, `DeleteListener` (forward to target group). ALB: HTTP/HTTPS. NLB: TCP/TLS; HTTP/HTTPS rejected |
| Rules | `CreateRule`, `DescribeRules`, `DeleteRule` on application listeners only (path-pattern and/or host-header forward; priority ascending). Rejected on network listeners |
| Targets | `RegisterTargets`, `DescribeTargetHealth` (Lambda: `healthy` when a listener or rule forwards and permission Allows; IP/instance: `healthy` when a listener forwards — control-plane registration only) |
| Access logs | `ModifyLoadBalancerAttributes` / `DescribeLoadBalancerAttributes` for `access_logs.s3.enabled` / `access_logs.s3.bucket` / `access_logs.s3.prefix`; lab `/alb/` listener appends ALB access-log lite lines to an in-account S3 bucket when enabled |
| Lab listener | `GET`/`POST` `/alb/{accountId}/{loadBalancerName}/{port}[/{path...}]` invokes the first registered Lambda on the matched target group (ALB event shape). Network LBs have no HTTP/L4 lab dataplane |

### Network Load Balancer lite

`CreateLoadBalancer` with `Type=network` persists `Type` and returns a `loadbalancer/net/...` ARN. Pair with `CreateTargetGroup` (`TargetType=instance` or `ip`, `Protocol=TCP` or `TLS`) and `CreateListener` (`Protocol=TCP` or `TLS`). `DescribeTargetHealth` reflects registration when a listener forwards; there is no TCP/TLS proxy or `/nlb/` invoke path on `:4566`.

### Listener rules lite (application only)

`CreateRule` accepts `ListenerArn`, `Priority` (1–50000), `Conditions` with `Field=path-pattern` and/or `Field=host-header` plus `Values` (or `PathPatternConfig.Values` / `HostHeaderConfig.Values`), and `Actions` with `Type=forward` plus `TargetGroupArn`. At least one condition field type is required.

- Path patterns: exact `/foo` or prefix `/foo*` (single trailing `*` only).
- Host headers: exact hostname (case-insensitive; optional `:port` on the request Host is stripped) or prefix `host*` (single trailing `*` only).

Values within one field are OR'd. Path and host field types are AND'd when both are present. Duplicate priorities return `PriorityInUse`. Unknown target groups return `TargetGroupNotFound`.

Lab invoke resolves `routePath` and the request `Host` against rules for that listener port in ascending priority order; the first match wins. If no rule matches, the listener default target group is used. Same open-dataplane gate as Function URL `NONE`.

### Authz notes

Identity `EvaluateFull` on `elasticloadbalancing:*`.

Lambda targets must resolve to an existing function and Allow `elasticloadbalancing.amazonaws.com` on the function resource policy (`lambda:AddPermission`; optional `SourceArn` = target group ARN). IP targets require a parseable lab IP. Instance targets require an `i-...` id (no EC2 API).

The lab listener is an open dataplane path gated like Function URL `NONE`: allowed on loopback listen by default; non-loopback requires `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1`. Associated WAFv2 Web ACLs on the load balancer ARN are enforced on invoke. WAFv2 association accepts ALB `loadbalancer/app/...` only (NLB `net` ARNs stay rejected until an enforce path exists).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

### Application LB + rules

```bash
aws elbv2 create-load-balancer --name lab-alb --endpoint-url "$EP"
aws elbv2 create-target-group --name lab-tg-a --target-type lambda --endpoint-url "$EP"
aws elbv2 create-target-group --name lab-tg-b --target-type lambda --endpoint-url "$EP"
aws lambda add-permission \
  --function-name lab-fn-a \
  --statement-id elb-invoke-a \
  --action lambda:InvokeFunction \
  --principal elasticloadbalancing.amazonaws.com \
  --source-arn "$TG_A_ARN" \
  --endpoint-url "$EP"
aws lambda add-permission \
  --function-name lab-fn-b \
  --statement-id elb-invoke-b \
  --action lambda:InvokeFunction \
  --principal elasticloadbalancing.amazonaws.com \
  --source-arn "$TG_B_ARN" \
  --endpoint-url "$EP"
aws elbv2 register-targets --target-group-arn "$TG_A_ARN" --targets Id="$FN_A_ARN" --endpoint-url "$EP"
aws elbv2 register-targets --target-group-arn "$TG_B_ARN" --targets Id="$FN_B_ARN" --endpoint-url "$EP"
aws elbv2 create-listener \
  --load-balancer-arn "$LB_ARN" \
  --protocol HTTP --port 80 \
  --default-actions Type=forward,TargetGroupArn="$TG_B_ARN" \
  --endpoint-url "$EP"
aws elbv2 create-rule \
  --listener-arn "$LISTENER_ARN" \
  --priority 5 \
  --conditions Field=path-pattern,Values='/api*' \
  --actions Type=forward,TargetGroupArn="$TG_A_ARN" \
  --endpoint-url "$EP"
aws elbv2 create-rule \
  --listener-arn "$LISTENER_ARN" \
  --priority 10 \
  --conditions Field=host-header,Values='api.example.com' \
  --actions Type=forward,TargetGroupArn="$TG_A_ARN" \
  --endpoint-url "$EP"
aws elbv2 describe-rules --listener-arn "$LISTENER_ARN" --endpoint-url "$EP"
curl -sS -X POST "http://127.0.0.1:4566/alb/$ACCOUNT/lab-alb/80/api/x" -d '{"ok":true}'
curl -sS -H 'Host: api.example.com' -X POST "http://127.0.0.1:4566/alb/$ACCOUNT/lab-alb/80/other" -d '{"ok":true}'
curl -sS -X POST "http://127.0.0.1:4566/alb/$ACCOUNT/lab-alb/80/other" -d '{"ok":true}'
```

Expect `/api/x` and `Host: api.example.com` to hit TG-A, and unmatched requests to hit default TG-B. Live `curl` needs healthy DinD (`noctaxris-engine`); unit tests cover path/host match with an invoke hook. Skip live smoke when Docker is unavailable.

### Network LB (control plane)

```bash
aws elbv2 create-load-balancer --name lab-nlb --type network --endpoint-url "$EP"
aws elbv2 describe-load-balancers --names lab-nlb --endpoint-url "$EP"
aws elbv2 create-target-group --name lab-nlb-tg --target-type instance --protocol TCP --port 80 --endpoint-url "$EP"
aws elbv2 register-targets --target-group-arn "$NLB_TG_ARN" --targets Id=i-labdeadbeef01,Port=80 --endpoint-url "$EP"
aws elbv2 create-listener \
  --load-balancer-arn "$NLB_ARN" \
  --protocol TCP --port 80 \
  --default-actions Type=forward,TargetGroupArn="$NLB_TG_ARN" \
  --endpoint-url "$EP"
aws elbv2 describe-target-health --target-group-arn "$NLB_TG_ARN" --endpoint-url "$EP"
aws elbv2 delete-load-balancer --load-balancer-arn "$NLB_ARN" --endpoint-url "$EP"
```

Expect `Type=network`, a `loadbalancer/net/...` ARN, and `healthy` target health after the listener is attached. There is no `/nlb/` or TCP dataplane on `:4566`.

## Not yet / deferred

- ALB Cognito authenticate action
- HTTP-header / query-string / source-ip conditions; multi-action rules
- NLB L4 proxy / lab TCP invoke path
- IP / instance target dataplane (beyond control-plane health)
- Weighted target groups
- UDP / TCP_UDP / QUIC NLB listeners
- Gateway Load Balancer (`Type=gateway`)
