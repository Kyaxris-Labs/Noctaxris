# RDS Data API

**Status:** shipped (lab core)

HTTPS Data API on `:4566` for `ExecuteStatement`. Requires `resourceArn` (RDS DB instance ARN) and `secretArn` (Secrets Manager). Begin/Commit/Rollback return 501 until nested SQL transactions exist.

## Implemented

| Area | Actions |
|------|---------|
| Statements | `ExecuteStatement` (prefers `pgx` against the nested data-plane DSN; falls back to nested `psql`) |
| Authz | Identity `EvaluateFull` on `rds-data:*` |
| Secrets | Rejects missing or mismatched `secretArn` (fail closed) |
| Unavailable | No nested Postgres → `DatabaseUnavailableException` (no canned SELECT success) |
| Result shape | `pgx`: OID-mapped fields (boolean / long / double / string / blob). nested-psql: SELECT cells as `stringValue` / `VARCHAR`; DML uses command-tag update counts |
| Parameters | Named `:name` binds via `pgx` when the nested DSN dials; nested-psql fallback rewrites to typed SQL literals |

### Executor selection

| Condition | Result | Marker in `formattedRecords` |
|-----------|--------|------------------------------|
| DinD up, instance `available` with container, nested hostname dialable from the API | Real SQL via in-process `pgx` (typed fields + bind parameters) | `noctaxrisExecutor=pgx` |
| DinD up and nested container present, but nested hostname not reachable from the API | Real SQL via `psql` **inside** the nested container (Docker exec; no host DB port) | `noctaxrisExecutor=nested-psql` |
| `NOCTAXRIS_RDS_DATA_PGX=0` | Force nested-psql (skip wire dial) | `noctaxrisExecutor=nested-psql` |
| No DinD / instance still `creating` / empty container | `DatabaseUnavailableException` | n/a |

The wire DSN is built only from the nested data-plane endpoint (`noctaxris-data-rds-<id>:5432`) plus Secrets Manager master credentials. Loopback, raw IPs, and host-published ports are rejected. Nested SQL reuses the existing DinD TLS client for the psql fallback. Unit tests may inject a stub executor; production paths do not fake SELECT success.

Supported parameter value shapes: `stringValue`, `longValue`, `doubleValue`, `booleanValue`, `blobValue` (base64), `isNull`.

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

aws rds-data execute-statement \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --database postgres \
  --sql 'SELECT :id::bigint AS n' \
  --parameters '[{"name":"id","value":{"longValue":7}}]' \
  --endpoint-url "$EP"
```

When Compose includes `noctaxris-engine` and the instance reaches `available` with a nested container, expect `noctaxrisExecutor=pgx` or `noctaxrisExecutor=nested-psql` (API processes are not on the DinD-internal `noctaxris-data` network, so nested-psql is the usual Compose path). Without DinD, expect `DatabaseUnavailableException`. Full nested harness: [docker/smoke-nested.sh](../../docker/smoke-nested.sh).

Optional live unit smoke against any Postgres reachable to the test process:

```bash
NOCTAXRIS_TEST_PGX_DSN='postgres://USER:PASS@HOST:5432/DB?sslmode=disable' \
  go test ./internal/server/ -count=1 -run TestExecuteRDSDataPgxWithConnLive
```

## Not yet / deferred

- `BatchExecuteStatement`, `ExecuteSql` legacy
- Full result type matrix and `formatRecordsAs=JSON`
- Real SQL transactions (`BeginTransaction` / `CommitTransaction` / `RollbackTransaction` return 501)
