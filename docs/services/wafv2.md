# WAF v2

**Status:** shipped (lab core)

Web ACL and rule group shape lite, AssociateWebACL with a lab resource ARN string, invoke-path enforcement for associated ACLs (default-action and ByteMatch rules), and a cheap Evaluate helper for labeled allow/block rules. Identity authz. No real edge PoP / full statement catalog.

## Implemented

| Area | Actions |
|------|---------|
| Web ACL | `CreateWebACL`, `UpdateWebACL`, `GetWebACL`, `ListWebACLs` |
| Rule group | `CreateRuleGroup` |
| Association | `AssociateWebACL` (HTTP API / execute-api / AppSync / Lambda function ARNs / ALB `loadbalancer/app/...` with an invoke gate; Web ACL must exist in-account; fail closed on unknown or unenforced shapes) |
| Invoke gate | Associated Web ACL rules and DefaultAction on HTTP API, AppSync GraphQL, Function URL, and ELB lab listener invoke (evaluate errors fail closed with 403) |
| Lab helper | `Evaluate` (label match and optional URI/headers for ByteMatch) |

Rules may use a `Label` string on the Evaluate helper, or a `Statement.ByteMatchStatement` on invoke (and Evaluate when URI/headers are supplied). Invoke enforcement passes request path and allowlisted headers (`Host`, `User-Agent`, `X-Forwarded-For`). Rules run in priority order; first match wins; otherwise DefaultAction applies. DefaultAction is Allow or Block.

### ByteMatch statement subset (lab)

| Field | Supported values |
|-------|------------------|
| `FieldToMatch` | `UriPath`, `SingleHeader` (`Name` required) |
| `PositionalConstraint` | `CONTAINS`, `EXACTLY` |
| `SearchString` | Plain text in lab JSON; standard base64 also accepted |

`AssociateWebACL` accepts lab HTTP API ARNs shaped like `arn:aws:apigateway:REGION::/apis/APIID[/stages/STAGE]`, `arn:aws:execute-api:...`, AppSync API ARNs, Lambda function ARNs for Function URL labs, and Application LB ARNs (`arn:aws:elasticloadbalancing:...:loadbalancer/app/...`) for the ELB lab listener. API Gateway REST (`restapis/...`), NLB `loadbalancer/net/...`, and Cognito user-pool ARNs are rejected until an enforce path exists. Unknown resource ARN services fail closed. Phantom Web ACL ARNs are rejected.

### Authz notes

Identity `EvaluateFull` on `wafv2:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws wafv2 create-web-acl \
  --name lab-acl --scope REGIONAL \
  --default-action Allow={} \
  --visibility-config SampledRequestsEnabled=true,CloudWatchMetricsEnabled=false,MetricName=lab \
  --endpoint-url "$EP"
```

CLI smoke for Evaluate uses the JSON target API against the lab endpoint after CreateWebACL returns an ARN.

Associate to a lab HTTP API ARN after Gateway creates an API:

```bash
aws wafv2 associate-web-acl \
  --web-acl-arn "$WEB_ACL_ARN" \
  --resource-arn "arn:aws:apigateway:us-east-1::/apis/$API_ID/stages/\$default" \
  --endpoint-url "$EP"
```

A Block default action or matching ByteMatch rule on that association returns HTTP 403 on `/http-api/...` invoke.

## Not yet / deferred

- Real edge PoP / CAPTCHA / Bot Control beyond supported rules
- Full WAF statement catalog
