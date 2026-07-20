# Resource Groups Tagging API

**Status:** shipped (lab core)

Lab TagResources / UntagResources / GetResources over a central SQLite tag map keyed by resource ARN. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Tags | `TagResources`, `UntagResources` |
| Query | `GetResources` with optional `TagFilters` and `ResourceTypeFilters` |

Tags live in a lab `resource_tags` table. They are not yet mirrored into every service-native tagging API. ARNs must belong to the caller account. S3 bucket ARNs without an account id are accepted for the caller.

### Authz notes

Identity `EvaluateFull` on `tag:TagResources`, `tag:UntagResources`, and `tag:GetResources` with resource `*`. Org SCP/RCP filters apply.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
ARN="arn:aws:sqs:us-east-1:000000000001:noctaxris-tag-lab"

aws resourcegroupstaggingapi tag-resources \
  --resource-arn-list "$ARN" \
  --tags env=lab,team=core \
  --endpoint-url "$EP"

aws resourcegroupstaggingapi get-resources \
  --tag-filters Key=env,Values=lab \
  --endpoint-url "$EP"

aws resourcegroupstaggingapi untag-resources \
  --resource-arn-list "$ARN" \
  --tag-keys team \
  --endpoint-url "$EP"
```

## Not yet / deferred

- Full Resource Groups CRUD and GroupBy
- Tag policy compliance details on GetResources
- Service-native tag APIs for every resource type
- Every AWS resource type filter matrix
