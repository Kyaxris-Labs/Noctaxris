# WAF v2

**Status:** shipped (lab core)

Web ACL and rule group shape lite, AssociateWebACL with a lab resource ARN string, invoke-path enforcement for associated ACLs (default-action gate), and a cheap Evaluate helper for labeled allow/block rules. Identity authz. No real edge PoP / full statement catalog.

## Implemented

| Area | Actions |
|------|---------|
| Web ACL | `CreateWebACL`, `UpdateWebACL`, `GetWebACL`, `ListWebACLs` |
| Rule group | `CreateRuleGroup` |
| Association | `AssociateWebACL` (HTTP API / execute-api / AppSync / Lambda function ARNs with an invoke gate; Web ACL must exist in-account; fail closed on unknown or unenforced shapes) |
| Invoke gate | Associated Web ACL DefaultAction on HTTP API, AppSync GraphQL, and Function URL invoke (evaluate errors fail closed with 403) |
| Lab helper | `Evaluate` (label match Allow/Block; invoke path does not supply request labels) |

Rules use a `Label` string match on the Evaluate helper. Invoke enforcement uses DefaultAction only (empty request label). DefaultAction is Allow or Block.

`AssociateWebACL` accepts lab HTTP API ARNs shaped like `arn:aws:apigateway:REGION::/apis/APIID[/stages/STAGE]`, `arn:aws:execute-api:...`, AppSync API ARNs, and Lambda function ARNs for Function URL labs. ALB, API Gateway REST (`restapis/...`), and Cognito user-pool ARNs are rejected until an enforce path exists. Unknown resource ARN services fail closed. Phantom Web ACL ARNs are rejected.

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

A Block default action on that association returns HTTP 403 on `/http-api/...` invoke.

## Not yet / deferred

- Real edge PoP / CAPTCHA / Bot Control beyond DefaultAction + label rules
- Full WAF statement catalog
