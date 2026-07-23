# RDS Data API

**Status:** shipped (lab core)

HTTPS Data API on `:4566` for `ExecuteStatement`. Requires `resourceArn` (RDS DB instance ARN) and `secretArn` (Secrets Manager). Begin/Commit/Rollback return 501 until nested SQL transactions exist.

## Implemented

| Area | Actions |
|------|---------|
| Statements | `ExecuteStatement` (nested `psql` when instance is `available` with a container) |
| Authz | Identity `EvaluateFull` on `rds-data:*` |
| Secrets | Rejects missing or mismatched `secretArn` (fail closed) |
| Unavailable | No nested Postgres → `DatabaseUnavailableException` (no canned SELECT success) |
| Result shape | SELECT cells as `stringValue` / `VARCHAR`; DML uses parsed command-tag update counts when present |
| Parameters | Request field parsed; rejected until optional wire-protocol executor is enabled |

### Nested SQL (default path)

| Condition | Result | Marker in `formattedRecords` |
|-----------|--------|------------------------------|
| DinD up and CreateDBInstance started nested Postgres (`available` + container) | Real SQL via `psql` **inside** the nested container (Docker exec; no host DB port) | `noctaxrisExecutor=nested-psql` |
| No DinD / instance still `creating` / empty container | `DatabaseUnavailableException` | n/a |

Default executor is nested `psql`. There is no `pgx` (or other Postgres wire driver) in `go.mod`. Nested SQL reuses the existing DinD TLS client. Unit tests may inject a stub executor; production paths do not fake SELECT success.

`parameters` / bind variables need a wire-protocol executor. Until that optional dependency is approved and linked, ExecuteStatement with a non-empty `parameters` array returns `BadRequestException`. Nested-psql remains VARCHAR-only for SELECT cells.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Create an RDS instance first (see [rds.md](rds.md)), wait until status is `available`, then:

```bash
# Replace ARNs from create-db-instance / describe output
aws rds-data execute-statement \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --database postgres \
  --sql 'SELECT 1' \
  --endpoint-url "$EP"
```

When Compose includes `noctaxris-engine` and the instance reaches `available` with a nested container, expect `noctaxrisExecutor=nested-psql`. Without DinD, expect `DatabaseUnavailableException`. Full nested harness: [docker/smoke-nested.sh](../../docker/smoke-nested.sh).

## Not yet / deferred

- Wire-protocol `pgx` executor (typed OID field mapping + bind parameters; needs explicit dependency buy-in; reserved flag `NOCTAXRIS_RDS_DATA_PGX` does nothing until then)
- `BatchExecuteStatement`, `ExecuteSql` legacy
- Full result type matrix and `formatRecordsAs=JSON`
- Real SQL transactions (`BeginTransaction` / `CommitTransaction` / `RollbackTransaction` return 501)
