# RDS Data API

**Status:** shipped (lab core)

HTTPS Data API on `:4566` for `ExecuteStatement`, `BatchExecuteStatement`, and real SQL transactions (`BeginTransaction` / `CommitTransaction` / `RollbackTransaction`). Requires `resourceArn` (RDS DB instance ARN) and `secretArn` (Secrets Manager).

## Implemented

| Area | Actions |
|------|---------|
| Statements | `ExecuteStatement` (prefers `pgx` against the nested data-plane DSN; falls back to nested `psql`) |
| Batch | `BatchExecuteStatement` (one SQL run per `parameterSets` entry; empty `parameterSets` → `BadRequestException`) |
| Transactions | `BeginTransaction` / `CommitTransaction` / `RollbackTransaction` via a held `pgx` session (process-local); SQLite stores metadata only |
| Execute in txn | `ExecuteStatement` / `BatchExecuteStatement` with `transactionId` use the held `pgx` conn (no nested-psql for txn-scoped SQL) |
| Authz | Identity `EvaluateFull` on `rds-data:*` |
| Secrets | Rejects missing or mismatched `secretArn` (fail closed) |
| Unavailable | No nested Postgres / pgx dial failure for Begin or txn sessions → `DatabaseUnavailableException` (no canned SELECT success) |
| Result shape | `pgx`: OID-mapped fields (boolean / long / double / string / blob). nested-psql: SELECT cells as `stringValue` / `VARCHAR`; DML uses command-tag update counts. Batch returns AWS-shaped `updateResults[].generatedFields` |
| Parameters | Named `:name` binds via `pgx` when the nested DSN dials; nested-psql fallback rewrites to typed SQL literals |

### Executor selection

| Condition | Result | Marker in `formattedRecords` |
|-----------|--------|------------------------------|
| DinD up, instance `available` with container, nested hostname dialable from the API | Real SQL via in-process `pgx` (typed fields + bind parameters) | `noctaxrisExecutor=pgx` |
| DinD up and nested container present, but nested hostname not reachable from the API | Real SQL via `psql` **inside** the nested container (Docker exec; no host DB port). Auto-commit `ExecuteStatement` / `BatchExecuteStatement` only | `noctaxrisExecutor=nested-psql` |
| `NOCTAXRIS_RDS_DATA_PGX=0` | Force nested-psql for auto-commit Execute/Batch (skip wire dial). Transactions still require `pgx` | `noctaxrisExecutor=nested-psql` |
| No DinD / instance still `creating` / empty container | `DatabaseUnavailableException` | n/a |
| Begin / Execute with `transactionId` when nested wire DSN cannot be held | `DatabaseUnavailableException` or `TransactionNotFoundException` | n/a |

Transactions keep a held `pgx` connection after real SQL `BEGIN`. Idle timeout is 5 minutes (lab); expired ids are rolled back best-effort and then return `TransactionNotFoundException`. Nested-psql cannot hold a session, so Begin fails closed when the API process cannot dial the nested data-plane hostname.

The wire DSN is built only from the nested data-plane endpoint (`noctaxris-data-rds-<id>:5432`) plus Secrets Manager master credentials. Loopback, raw IPs, and host-published ports are rejected. Nested SQL reuses the existing DinD TLS client for the psql fallback. Unit tests may inject a stub executor for auto-commit Execute; production paths do not fake SELECT success. Stub overrides do not create SQL transactions.

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

# Batch (auto-commit). Requires nested engine reachable for SQL.
aws rds-data batch-execute-statement \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --database postgres \
  --sql 'SELECT 1' \
  --parameter-sets '[[],[]]' \
  --endpoint-url "$EP"

# Transactions require a held pgx session (API process must dial nested hostname).
TX=$(aws rds-data begin-transaction \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --database postgres \
  --endpoint-url "$EP" \
  --query transactionId --output text)
aws rds-data execute-statement \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --database postgres \
  --transaction-id "$TX" \
  --sql 'SELECT 1' \
  --endpoint-url "$EP"
aws rds-data commit-transaction \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --transaction-id "$TX" \
  --endpoint-url "$EP"
```

When Compose includes `noctaxris-engine` and the instance reaches `available` with a nested container, auto-commit Execute usually uses nested-psql (API processes are not on the DinD-internal `noctaxris-data` network). BeginTransaction needs the API process to dial that nested hostname with `pgx`; otherwise expect `DatabaseUnavailableException`. Full nested harness: [docker/smoke-nested.sh](../../docker/smoke-nested.sh).

Optional live unit smoke against any Postgres reachable to the test process:

```bash
NOCTAXRIS_TEST_PGX_DSN='postgres://USER:PASS@HOST:5432/DB?sslmode=disable' \
  go test ./internal/server/ -count=1 -run 'TestExecuteRDSDataPgxWithConnLive|TestRDSDataTxnLive'
```

## Not yet / deferred

- `ExecuteSql` legacy
- Full result type matrix and `formatRecordsAs=JSON`
- Batch `generatedFields` population from `RETURNING` / identity columns
- AWS 3-minute idle timeout (lab uses 5 minutes)
- Cross-process transaction resume (held sessions are process-local)
