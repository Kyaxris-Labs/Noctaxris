# Athena

**Status:** shipped (lab core)

In-process SELECT subset over Glue Data Catalog tables and lab S3 CSV or JSON objects. Identity authz. No nested Trino or Presto.

## Implemented

| Area | Actions |
|------|---------|
| Query | `StartQueryExecution`, `GetQueryExecution`, `GetQueryResults`, `StopQueryExecution` |
| SQL subset | `SELECT cols FROM db.table [LIMIT n]` (or `table` with `QueryExecutionContext.Database`) |
| Catalog | Resolves tables from Glue (`StorageDescriptor.Location`, columns, SerDe/InputFormat for CSV vs JSON) |
| Results | In-memory result set. Optional `ResultConfiguration.OutputLocation` writes CSV under lab S3; write failures mark the query `FAILED` |
| WorkGroup | Optional. Defaults to `primary` |

Unsupported SQL fails with `InvalidRequestException` or a `FAILED` query execution (not an empty success). Missing Glue tables fail with `TABLE_NOT_FOUND`. Missing S3 location buckets fail closed with `FAILED` (not empty SUCCEEDED). If a listed object under the table prefix fails `GetObject`, the query is `FAILED` (no silent skip). An empty prefix listing that succeeds may return header-only `SUCCEEDED` (intentional when no objects match).

### Authz notes

Identity `EvaluateFull` on `athena:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws s3 mb s3://athena-lab --endpoint-url "$EP"
echo -e 'id,name\n1,alice\n2,bob' | aws s3 cp - s3://athena-lab/data/t1.csv --endpoint-url "$EP"

aws glue create-database --database-input Name=labdb --endpoint-url "$EP"
aws glue create-table --database-name labdb --table-input '{
  "Name":"people",
  "StorageDescriptor":{
    "Location":"s3://athena-lab/data/",
    "Columns":[{"Name":"id","Type":"string"},{"Name":"name","Type":"string"}],
    "SerdeInfo":{"SerializationLibrary":"org.apache.hadoop.hive.serde2.lazy.LazySimpleSerDe","Parameters":{"field.delim":","}}
  }
}' --endpoint-url "$EP"

QID=$(aws athena start-query-execution \
  --query-string "SELECT id, name FROM labdb.people LIMIT 10" \
  --query-execution-context Database=labdb \
  --result-configuration OutputLocation=s3://athena-lab/results/ \
  --endpoint-url "$EP" --query QueryExecutionId --output text)

aws athena get-query-execution --query-execution-id "$QID" --endpoint-url "$EP"
aws athena get-query-results --query-execution-id "$QID" --endpoint-url "$EP"
```

Skip live Compose smoke when Docker is unavailable. Store and server unit tests cover the lab path without DinD.

## Not yet / deferred

- Full SQL, CTAS, UNLOAD, INSERT, federated catalogs
- WorkGroup configuration matrix and result reuse
- Nested Trino / Presto / Spark engines
- Managed query results encryption options
