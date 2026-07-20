# Pricing

**Status:** shipped (lab core)

DescribeServices, GetAttributeValues, and GetProducts over a tiny static embedded price list (EC2, S3, Lambda sample SKUs). Identity authz. No live AWS Price List sync.

## Implemented

| Area | Actions |
|------|---------|
| Catalog | `DescribeServices`, `GetAttributeValues`, `GetProducts` |

### Authz notes

Identity `EvaluateFull` on `pricing:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws pricing describe-services --endpoint-url "$EP"
aws pricing get-attribute-values --service-code AmazonEC2 --attribute-name instanceType --endpoint-url "$EP"
aws pricing get-products --service-code AmazonS3 --endpoint-url "$EP"
```

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Live AWS Price List sync
- Cost Explorer, CUR, Budgets
