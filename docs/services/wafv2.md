# WAF v2

**Status:** shipped (lab core)

Web ACL and rule group shape lite, AssociateWebACL with a lab resource ARN string, and a cheap Evaluate helper for labeled allow/block rules. Identity authz. No real edge PoP.

## Implemented

| Area | Actions |
|------|---------|
| Web ACL | `CreateWebACL`, `UpdateWebACL`, `GetWebACL`, `ListWebACLs` |
| Rule group | `CreateRuleGroup` |
| Association | `AssociateWebACL` |
| Lab helper | `Evaluate` (label match Allow/Block) |

Rules use a `Label` string match. DefaultAction is Allow or Block.

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

## Not yet / deferred

- Real CloudFront / ALB association
- Bot Control, CAPTCHA, full statement catalog
