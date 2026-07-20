# CloudFront

**Status:** shipped (lab core)

Distribution CRUD lite. Origins are S3 bucket name strings or API Gateway API id strings. Identity authz. No real PoP or WAN edge.

## Implemented

| Area | Actions |
|------|---------|
| Distribution | `CreateDistribution`, `GetDistribution`, `ListDistributions`, `DeleteDistribution` |
| Origins | `OriginType` `s3` or `apigateway` (DomainName is a lab string) |

### Authz notes

Identity `EvaluateFull` on `cloudfront:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws cloudfront create-distribution \
  --distribution-config file://dist.json \
  --endpoint-url "$EP"
```

Lab JSON target API also works when the CLI shape is awkward. Skip live smoke when Docker is unavailable (unit tests cover CRUD).

## Not yet / deferred

- Real CloudFront PoPs and cache behaviors matrix
- Signed cookies / URLs depth
- Custom domain ACM linkage beyond string fields
- WAF association enforcement at a fake edge
