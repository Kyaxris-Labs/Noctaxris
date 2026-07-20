# RDS Data API

**Status:** shipped (lab core, stub executor)

HTTPS Data API on `:4566` for `ExecuteStatement` and Begin/Commit/Rollback lite. Requires `resourceArn` (RDS DB instance ARN) and `secretArn` (Secrets Manager). Unit tests use a recorded-statement stub executor. Live Postgres wire protocol (`pgx`) is not in `go.mod` yet.

## Implemented

| Area | Actions |
|------|---------|
| Statements | `ExecuteStatement` |
| Transactions | `BeginTransaction`, `CommitTransaction`, `RollbackTransaction` |
| Authz | Identity `EvaluateFull` on `rds-data:*` |
| Secrets | Rejects missing or mismatched `secretArn` (fail closed) |

### Stub vs live SQL

The default executor records SQL and returns canned SELECT-shaped records. It validates ARNs and authz without Docker. A live nested Postgres driver may be added later with explicit dependency buy-in.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Create an RDS instance first (see [rds.md](rds.md)), then:

```bash
# Replace ARNs from create-db-instance / describe output
aws rds-data execute-statement \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --database postgres \
  --sql 'SELECT 1' \
  --endpoint-url "$EP"
```

Skip live nested smoke when Docker/DinD is unavailable. Stub path still works in unit tests.

## Not yet / deferred

- Live `pgx` executor against nested Postgres (needs explicit dependency buy-in)
- `BatchExecuteStatement`, `ExecuteSql` legacy
- Full result type matrix and `formatRecordsAs=JSON`
