# RDS Data API

**Status:** shipped (lab core)

HTTPS Data API on `:4566` for `ExecuteStatement` and Begin/Commit/Rollback lite. Requires `resourceArn` (RDS DB instance ARN) and `secretArn` (Secrets Manager).

## Implemented

| Area | Actions |
|------|---------|
| Statements | `ExecuteStatement` |
| Transactions | `BeginTransaction`, `CommitTransaction`, `RollbackTransaction` |
| Authz | Identity `EvaluateFull` on `rds-data:*` |
| Secrets | Rejects missing or mismatched `secretArn` (fail closed) |

### Stub vs nested SQL

| Condition | Executor | Marker in `formattedRecords` |
|-----------|----------|------------------------------|
| No DinD, or RDS instance has no nested container | Recorded-statement **stub** (canned SELECT shape) | `noctaxrisExecutor=stub` |
| DinD up and CreateDBInstance started nested Postgres | Real SQL via `psql` **inside** the nested container (Docker exec; no host DB port) | `noctaxrisExecutor=nested-psql` |

There is no `pgx` (or other Postgres wire driver) in `go.mod`. Nested SQL reuses the existing DinD TLS client. Unit tests without Docker stay on the stub path.

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

When Compose includes `noctaxris-engine` and the instance reaches `available` with a nested container, expect `noctaxrisExecutor=nested-psql`. Without DinD, the stub marker is expected. Full nested harness: [docker/smoke-nested.sh](../../docker/smoke-nested.sh).

## Not yet / deferred

- Wire-protocol `pgx` executor (needs explicit dependency buy-in)
- `BatchExecuteStatement`, `ExecuteSql` legacy
- Full result type matrix and `formatRecordsAs=JSON`
- Real SQL transactions (Begin/Commit/Rollback remain control-plane lite)