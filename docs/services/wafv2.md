# WAF v2

**Status:** shipped (lab core)

Web ACL and rule group shape lite, IPSet CRUD, AssociateWebACL with a lab resource ARN string, invoke-path enforcement for associated ACLs (default-action, ByteMatch, SizeConstraint, and IPSet rules with inline Addresses or IPSet ARN), and a cheap Evaluate helper for labeled allow/block rules. Identity authz. No real edge PoP / full statement catalog.

## Implemented

| Area | Actions |
|------|---------|
| Web ACL | `CreateWebACL`, `UpdateWebACL`, `GetWebACL`, `ListWebACLs` |
| IP set | `CreateIPSet`, `GetIPSet`, `UpdateIPSet`, `DeleteIPSet`, `ListIPSets` |
| Rule group | `CreateRuleGroup` |
| Association | `AssociateWebACL` (HTTP API / execute-api / AppSync / Lambda function ARNs / ALB `loadbalancer/app/...` with an invoke gate; Web ACL must exist in-account; fail closed on unknown or unenforced shapes) |
| Invoke gate | Associated Web ACL rules and DefaultAction on HTTP API, AppSync GraphQL, Function URL, and ELB lab listener invoke (evaluate errors fail closed with 403) |
| Lab helper | `Evaluate` (label match and optional URI/headers/SourceIP for statements) |

Rules may use a `Label` string on the Evaluate helper, or a statement on invoke (and Evaluate when URI/headers/SourceIP are supplied): `Statement.ByteMatchStatement`, `Statement.SizeConstraintStatement`, or `Statement.IPSetReferenceStatement` with `ARN` and/or lab inline `Addresses`. Invoke enforcement passes request path, allowlisted headers (`Host`, `User-Agent`, `X-Forwarded-For`), and `SourceIP` from the TCP peer by default. The first parseable `X-Forwarded-For` hop is used only when the peer is covered by `NOCTAXRIS_TRUSTED_PROXIES` (empty default ignores XFF). Rules run in priority order; first match wins; otherwise DefaultAction applies. DefaultAction is Allow or Block.

### ByteMatch statement subset (lab)

| Field | Supported values |
|-------|------------------|
| `FieldToMatch` | `UriPath`, `SingleHeader` (`Name` required) |
| `PositionalConstraint` | `CONTAINS`, `EXACTLY` |
| `SearchString` | Plain text in lab JSON; standard base64 also accepted |

### SizeConstraint statement subset (lab)

| Field | Supported values |
|-------|------------------|
| `FieldToMatch` | `UriPath`, `SingleHeader` (`Name` required) |
| `ComparisonOperator` | `EQ`, `NE`, `LE`, `LT`, `GE`, `GT` |
| `Size` | Non-negative integer; compared to byte length of the matched field |

### IPSet and IPSetReference statement (lab)

| Area | Behavior |
|------|----------|
| IPSet resource | `Name`, `Id`, `ARN`, `Scope` (`REGIONAL` / `CLOUDFRONT`), `IPAddressVersion` (`IPV4` / `IPV6`), `Addresses` (CIDRs), `LockToken` |
| `IPSetReferenceStatement.ARN` | Resolves stored IPSet addresses for `SourceIP` matching; unknown ARN fails closed (evaluate error → invoke 403) |
| `IPSetReferenceStatement.Addresses` | Lab inline CIDRs or host IPs (hosts treated as `/32` or `/128`); may be used without a stored IPSet |

Empty or unparseable `SourceIP` does not match. An empty resolved address list does not match.

`AssociateWebACL` accepts lab HTTP API ARNs shaped like `arn:aws:apigateway:REGION::/apis/APIID[/stages/STAGE]`, `arn:aws:execute-api:...`, AppSync API ARNs, Lambda function ARNs for Function URL labs, and Application LB ARNs (`arn:aws:elasticloadbalancing:...:loadbalancer/app/...`) for the ELB lab listener. API Gateway REST (`restapis/...`), NLB `loadbalancer/net/...`, and Cognito user-pool ARNs are rejected until an enforce path exists. Unknown resource ARN services fail closed. Phantom Web ACL ARNs are rejected.

### Authz notes

Identity `EvaluateFull` on `wafv2:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws wafv2 create-ip-set \
  --name lab-block --scope REGIONAL \
  --ip-address-version IPV4 \
  --addresses 192.0.2.0/24 \
  --endpoint-url "$EP"
```

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

A Block default action or matching ByteMatch / SizeConstraint / IPSet rule on that association returns HTTP 403 on `/http-api/...` invoke.

## Not yet / deferred

- Real edge PoP / CAPTCHA / Bot Control beyond supported rules
- Full WAF statement catalog (And/Or/Not, managed rule groups, rate-based, GeoMatch, IPSetForwardedIPConfig)
- Trusted-proxy allowlist for `X-Forwarded-For`: set `NOCTAXRIS_TRUSTED_PROXIES` (comma-separated CIDRs). When empty (default), invoke enforcement uses the TCP peer only. With a matching peer, the first parseable XFF hop is used for IPSet SourceIP.
