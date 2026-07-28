# RDS Data API

**Status:** shipped (lab core)

HTTPS Data API on `:4566` for `ExecuteStatement`, `BatchExecuteStatement`, and real SQL transactions (`BeginTransaction` / `CommitTransaction` / `RollbackTransaction`). Supports `formatRecordsAs=JSON` and Batch `generatedFields` from Postgres `RETURNING` via `pgx`. Requires `resourceArn` (RDS DB instance ARN) and `secretArn` (Secrets Manager).

**Engine support:** `postgres`, `mysql`, and `mariadb` when the matching nested DinD engine is `available`. Unsupported engines return `BadRequestException`. No nested container or unreachable wire path returns `DatabaseUnavailableException` (fail closed).

## Implemented

| Area | Actions |
|------|---------|
| Statements | `ExecuteStatement` (Postgres: `pgx` then nested `psql`; MySQL/MariaDB: `go-sql-driver/mysql` then nested `mysql` CLI) |
| Batch | `BatchExecuteStatement` (one SQL run per `parameterSets` entry; empty `parameterSets` → `BadRequestException`) |
| Transactions | `BeginTransaction` / `CommitTransaction` / `RollbackTransaction` via held wire sessions (`pgx` or `database/sql`); SQLite stores metadata only |
| Execute in txn | `ExecuteStatement` / `BatchExecuteStatement` with `transactionId` use the held connection (no nested CLI for txn-scoped SQL) |
| Authz | Identity `EvaluateFull` on `rds-data:*` |
| Secrets | Rejects missing or mismatched `secretArn` (fail closed) |
| Engine gate | `postgres`, `mysql`, `mariadb` only; other engines → `BadRequestException` |
| Unavailable | No nested engine / wire dial failure for Begin or txn sessions → `DatabaseUnavailableException` (no canned SELECT success) |
| Result shape | Wire drivers: typed fields where supported; nested CLI: SELECT cells as `stringValue` / `VARCHAR`. `formatRecordsAs=JSON` returns simplified row objects in `formattedRecords` (clears `records` / `columnMetadata`). Postgres Batch `RETURNING` populates `generatedFields` via `pgx` |
| Parameters | Named `:name` binds on wire paths; nested CLI fallbacks rewrite to typed SQL literals |

### Executor selection

| Condition | Result | Marker in `formattedRecords` |
|-----------|--------|------------------------------|
| Postgres, DinD up, nested hostname dialable | Real SQL via in-process `pgx` | `noctaxrisExecutor=pgx` |
| Postgres, container present, wire dial fails | Real SQL via nested `psql` (auto-commit only) | `noctaxrisExecutor=nested-psql` |
| MySQL/MariaDB, wire dialable | Real SQL via in-process `mysql` driver | `noctaxrisExecutor=mysql` |
| MySQL/MariaDB, wire dial fails | Real SQL via nested `mysql` CLI (auto-commit only) | `noctaxrisExecutor=nested-mysql` |
| `NOCTAXRIS_RDS_DATA_PGX=0` | Force nested CLI for auto-commit Execute/Batch (skip wire dial). Transactions still require wire sessions | nested CLI markers |
| No DinD / instance still `creating` / empty container | `DatabaseUnavailableException` | n/a |
| Begin / txn-scoped Execute when wire session cannot be held | `DatabaseUnavailableException` or `TransactionNotFoundException` | n/a |

Transactions keep a held wire connection after SQL `BEGIN`. Idle timeout is 5 minutes (lab); expired ids are rolled back best-effort and then return `TransactionNotFoundException`. Nested CLI cannot hold a session, so Begin fails closed when the API process cannot dial the nested data-plane hostname.

The wire DSN is built only from the nested data-plane endpoint (`noctaxris-data-rds-<id>:5432` or `:3306`) plus Secrets Manager master credentials. Loopback, raw IPs, and host-published ports are rejected. Unit tests may inject a stub executor for auto-commit Execute; production paths do not fake SELECT success.

Supported parameter value shapes: `stringValue`, `longValue`, `doubleValue`, `booleanValue`, `blobValue` (base64), `isNull`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Create an RDS instance first (see [rds.md](rds.md)), wait until status is `available`, then:

Current AWS CLI v2 / boto3 may call Smithy RPC-v2 paths (`POST /Execute`, `POST /BatchExecute`, … with `Content-Type: application/json`) instead of `X-Amz-Target: AmazonRDSDataService.*`. Both shapes are accepted.

```bash
# Postgres
aws rds-data execute-statement \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --database postgres \
  --sql 'SELECT 1' \
  --endpoint-url "$EP"

# MySQL (create-db-instance --engine mysql first)
aws rds-data execute-statement \
  --resource-arn "$MYSQL_DB_ARN" \
  --secret-arn "$MYSQL_SECRET_ARN" \
  --database appdb \
  --sql 'SELECT 1' \
  --endpoint-url "$EP"

aws rds-data execute-statement \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --database postgres \
  --sql 'SELECT :id::bigint AS n' \
  --parameters '[{"name":"id","value":{"longValue":7}}]' \
  --endpoint-url "$EP"

aws rds-data execute-statement \
  --resource-arn "$MYSQL_DB_ARN" \
  --secret-arn "$MYSQL_SECRET_ARN" \
  --database appdb \
  --sql 'SELECT :n' \
  --parameters '[{"name":"n","value":{"longValue":7}}]' \
  --endpoint-url "$EP"

aws rds-data execute-statement \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --database postgres \
  --sql 'SELECT 1 AS n' \
  --format-records-as JSON \
  --endpoint-url "$EP"
# formattedRecords is a JSON array of objects; records/columnMetadata empty

# Batch (auto-commit). Requires nested engine reachable for SQL.
aws rds-data batch-execute-statement \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --database postgres \
  --sql 'SELECT 1' \
  --parameter-sets '[[],[]]' \
  --endpoint-url "$EP"

# Transactions require a held wire session (API process must dial nested hostname).
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

When Compose includes `noctaxris-engine` and the instance reaches `available` with a nested container, auto-commit Execute often uses nested CLI (API processes are not on the DinD-internal `noctaxris-data` network). BeginTransaction needs the API process to dial that nested hostname; otherwise expect `DatabaseUnavailableException`. Full nested harness: [docker/smoke-nested.sh](../../docker/smoke-nested.sh).

Optional live unit smoke against any Postgres reachable to the test process:

```bash
NOCTAXRIS_TEST_PGX_DSN='postgres://USER:PASS@HOST:5432/DB?sslmode=disable' \
  go test ./internal/server/ -count=1 -run 'TestExecuteRDSDataPgxWithConnLive|TestRDSDataTxnLive'
```

Optional MySQL wire smoke:

```bash
NOCTAXRIS_TEST_MYSQL_DSN='user:pass@tcp(HOST:3306)/DB?parseTime=true' \
  go test ./internal/server/ -count=1 -run TestExecuteRDSDataMySQLWithConnLive
```

## Not yet / deferred

- Full result type matrix beyond OID-mapped / VARCHAR cells (array types, nested structures)
- MySQL/MariaDB `RETURNING` / Batch `generatedFields` parity with Postgres

## Out of lab scope

- `ExecuteSql` legacy (out of lab scope)
- AWS 3-minute idle timeout (out of lab scope; lab uses 5 minutes)
- Cross-process transaction resume (out of lab scope; held wire sessions are process-local)
