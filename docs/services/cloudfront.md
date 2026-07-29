# CloudFront

**Status:** shipped (lab core)

Distribution CRUD lite plus a loopback fake-edge fetch path. Origins must resolve to an existing lab S3 bucket name or HTTP API id. Create returns `Status=Deployed` and a lab `DomainName` string (`d{id}.cloudfront.noctaxris.local`). There is no real PoP, DNS, or WAN CDN.

## Implemented

| Area | Actions / paths |
|------|-----------------|
| Distribution | `CreateDistribution`, `GetDistribution`, `GetDistributionConfig`, `UpdateDistribution`, `ListDistributions`, `DeleteDistribution` |
| Config / ETag | `GetDistributionConfig` returns `DistributionConfig` + `ETag` (also `ETag` response header). `UpdateDistribution` requires matching `IfMatch` / `If-Match`; mutates modeled `Enabled`, origins, and cache behaviors; bumps `ETag` |
| Invalidations | `CreateInvalidation`, `GetInvalidation`, `ListInvalidations` (paths stored; `Status=Completed` immediately; theatre only, no cache purge) |
| Origins | `OriginType` `s3` or `apigateway` (DomainName must exist in-account; fail closed) |
| Cache behaviors | Optional `DefaultCacheBehavior.TargetOriginId` (`*` path) plus `CacheBehaviors.Items` with `PathPattern` → `TargetOriginId` (matched in list order; prefix `*` suffix lite) |
| Logging | Optional `Logging` on create/update (`Bucket` / `Prefix` / `Enabled`); edge GET appends tab-separated access-log lite lines to an in-account S3 bucket |
| Fake-edge | SigV4 `GET /cloudfront/{distributionId}/{objectKey...}` on `:4566` (origin selected by path pattern; first origin when no behaviors) |
| Edge fetch | S3 → in-store `GetObject`; apigateway → internal `/http-api/...` invoke (never dials arbitrary hosts) |

### Authz notes

Identity `EvaluateFull` on `cloudfront:*`. Fake-edge requires SigV4 with service `cloudfront` and `cloudfront:GetDistribution` on the distribution ARN. Disabled distributions return 403. Associated WAFv2 ACLs (when present) are evaluated on the edge request.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws cloudfront create-distribution \
  --distribution-config file://dist.json \
  --endpoint-url "$EP"

aws cloudfront get-distribution-config --id "$DIST_ID" --endpoint-url "$EP"

aws cloudfront update-distribution \
  --id "$DIST_ID" \
  --if-match "$ETAG" \
  --distribution-config file://dist-updated.json \
  --endpoint-url "$EP"

aws cloudfront create-invalidation \
  --distribution-id "$DIST_ID" \
  --paths "/index.html" "/images/*" \
  --endpoint-url "$EP"
```

Fake-edge fetch (SigV4; substitute distribution id and object key):

```bash
# Example with curl + AWS SigV4 tooling, or any SigV4 client signed for service=cloudfront:
# GET $EP/cloudfront/<DistributionId>/path/to/object
```

Skip live smoke when Docker is unavailable (unit tests cover Deployed DomainName, GetDistributionConfig ETag, UpdateDistribution IfMatch, invalidation Completed theatre, SigV4-required edge, S3 origin bytes, path-pattern origin selection, and disabled-distribution 403).

## Not yet / deferred

- Real CloudFront PoPs and full cache policy / TTL matrix
- Mid-path wildcards beyond trailing `*` prefix (e.g. `images/*.jpg`)
- Signed cookies / URLs depth
- Custom domain ACM linkage beyond string fields
- Actual edge cache purge from invalidations (status theatre only)
