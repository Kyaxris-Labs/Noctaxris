# Cost Explorer

**Status:** shipped (lab core)

GetCostAndUsage and GetCostForecast over seeded lab amounts. Identity authz. No live AWS Cost Explorer sync.

## Implemented

| Area | Actions |
|------|---------|
| Cost | `GetCostAndUsage`, `GetCostForecast` |

### Authz notes

Identity `EvaluateFull` on `ce:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws ce get-cost-and-usage \
  --time-period Start=2026-07-01,End=2026-08-01 \
  --granularity MONTHLY \
  --metrics UnblendedCost \
  --endpoint-url "$EP"
aws ce get-cost-forecast \
  --time-period Start=2026-08-01,End=2026-09-01 \
  --metric UNBLENDED_COST \
  --granularity MONTHLY \
  --endpoint-url "$EP"
```

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Live AWS CE sync
- Anomaly detection
- Rightsizing recommendations
- Full filter and GroupBy matrices
