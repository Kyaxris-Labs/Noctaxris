# Budgets

**Status:** shipped (lab core)

CreateBudget, DescribeBudget, DescribeBudgets, and DeleteBudget. Optional `NotificationsWithSubscribers` are stored; SNS subscriber ARNs receive one lab ACTUAL threshold Publish on CreateBudget (best-effort). Identity authz.

## Implemented

| Area | Actions |
|------|---------|
| Budgets | `CreateBudget`, `DescribeBudget`, `DescribeBudgets`, `DeleteBudget` |
| Notify | CreateBudget Publishes to SNS topics listed under `NotificationsWithSubscribers[].Subscribers` with `SubscriptionType=SNS` |

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
- Threshold evaluation against live Cost Explorer spend (notify is CreateBudget fan-out only)
