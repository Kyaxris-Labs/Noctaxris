# WAF v2

**Status:** shipped (lab core)

Web ACL and rule group shape lite, AssociateWebACL with a lab resource ARN string, and a cheap Evaluate helper for labeled allow/block rules. Identity authz. No real edge PoP.

## Implemented

| Area | Actions |
|------|---------|
| Web ACL | `CreateWebACL`, `UpdateWebACL`, `GetWebACL`, `ListWebACLs` |
| Rule group | `CreateRuleGroup` |
| Association | `AssociateWebACL` (HTTP API / execute-api / ALB / AppSync / Cognito ARNs, fail closed on unknown) |
| Lab helper | `Evaluate` (label match Allow/Block) |

Rules use a `Label` string match. DefaultAction is Allow or Block.

`AssociateWebACL` accepts lab HTTP API ARNs shaped like `arn:aws:apigateway:REGION::/apis/APIID[/stages/STAGE]` and `arn:aws:execute-api:...`. Unknown resource ARN services fail closed.

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

## Not yet / deferred

- Real edge PoP enforcement of associated Web ACLs
- Bot Control, CAPTCHA, full statement catalog
