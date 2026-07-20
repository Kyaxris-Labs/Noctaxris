# Budgets

**Status:** shipped (lab core)

CreateBudget, DescribeBudget, DescribeBudgets, and DeleteBudget. Optional notification stubs are stored only (no SNS fan-out). Identity authz.

## Implemented

| Area | Actions |
|------|---------|
| Budgets | `CreateBudget`, `DescribeBudget`, `DescribeBudgets`, `DeleteBudget` |

### Authz notes

Identity `EvaluateFull` on `budgets:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws budgets create-budget --account-id 000000000001 --budget file://budget.json --endpoint-url "$EP"
aws budgets describe-budget --account-id 000000000001 --budget-name lab-budget --endpoint-url "$EP"
aws budgets delete-budget --account-id 000000000001 --budget-name lab-budget --endpoint-url "$EP"
```

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Budget actions that mutate accounts
- RI/SP coverage budgets
- SNS publish for threshold alerts
