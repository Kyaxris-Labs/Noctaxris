# CloudFront

**Status:** shipped (lab core, control-plane stub)

Distribution CRUD lite. Origins must resolve to an existing lab S3 bucket name or HTTP API id. Identity authz. No real PoP or WAN edge. Create returns `Status=InProgress` and omits `DomainName` until a documented fake-edge path exists.

## Implemented

| Area | Actions |
|------|---------|
| Distribution | `CreateDistribution`, `GetDistribution`, `ListDistributions`, `DeleteDistribution` |
| Origins | `OriginType` `s3` or `apigateway` (DomainName must exist in-account; fail closed) |

### Authz notes

Identity `EvaluateFull` on `cloudfront:*`.

Missing buckets or API ids are rejected at Create. Status stays `InProgress` (not `Deployed`) because there is no PoP.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws cloudfront create-distribution \
  --distribution-config file://dist.json \
  --endpoint-url "$EP"
```

Lab JSON target API also works when the CLI shape is awkward. Skip live smoke when Docker is unavailable (unit tests cover CRUD, origin resolve, and non-Deployed status).

## Not yet / deferred

- Real CloudFront PoPs and cache behaviors matrix
- Fake-edge DomainName / Deployed status
- Signed cookies / URLs depth
- Custom domain ACM linkage beyond string fields
- WAF association enforcement at a fake edge
