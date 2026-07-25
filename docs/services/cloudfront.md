# CloudFront

**Status:** shipped (lab core)

Distribution CRUD lite plus a loopback fake-edge fetch path. Origins must resolve to an existing lab S3 bucket name or HTTP API id. Create returns `Status=Deployed` and a lab `DomainName` string (`d{id}.cloudfront.noctaxris.local`). There is no real PoP, DNS, or WAN CDN.

## Implemented

| Area | Actions / paths |
|------|-----------------|
| Distribution | `CreateDistribution`, `GetDistribution`, `ListDistributions`, `DeleteDistribution` |
| Origins | `OriginType` `s3` or `apigateway` (DomainName must exist in-account; fail closed) |
| Logging | Optional `Logging` on create (`Bucket` / `Prefix` / `Enabled`); edge GET appends tab-separated access-log lite lines to an in-account S3 bucket |
| Fake-edge | SigV4 `GET /cloudfront/{distributionId}/{objectKey...}` on `:4566` (first origin only) |
| Edge fetch | S3 → in-store `GetObject`; apigateway → internal `/http-api/...` invoke (never dials arbitrary hosts) |

### Authz notes

Identity `EvaluateFull` on `cloudfront:*`. Fake-edge requires SigV4 with service `cloudfront` and `cloudfront:GetDistribution` on the distribution ARN. Disabled distributions return 403. Associated WAFv2 ACLs (when present) are evaluated on the edge request.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws cloudfront create-distribution \
  --distribution-config file://dist.json \
  --endpoint-url "$EP"
```

Fake-edge fetch (SigV4; substitute distribution id and object key):

```bash
# Example with curl + AWS SigV4 tooling, or any SigV4 client signed for service=cloudfront:
# GET $EP/cloudfront/<DistributionId>/path/to/object
```

Skip live smoke when Docker is unavailable (unit tests cover Deployed DomainName, SigV4-required edge, S3 origin bytes, and disabled-distribution 403).

## Not yet / deferred

- Real CloudFront PoPs and cache behaviors matrix
- Multi-origin selection / path pattern routing
- Signed cookies / URLs depth
- Custom domain ACM linkage beyond string fields
