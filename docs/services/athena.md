# Athena

**Status:** shipped (lab core)

In-process SELECT subset over Glue Data Catalog tables and lab S3 CSV or JSON objects. Identity authz. No nested Trino or Presto.

## Implemented

| Area | Actions |
|------|---------|
| Query | `StartQueryExecution`, `GetQueryExecution`, `GetQueryResults`, `StopQueryExecution` |
| SQL subset | `SELECT cols FROM db.table [alias] [JOIN\|INNER JOIN db.t2 b ON a.x = b.x] [WHERE col = 'literal' \| col != 'literal' \| col <> 'literal' \| col IN ('a','b') \| col LIKE 'pat%' \| json_extract(col,'$.path') = 'v'] [GROUP BY col] [ORDER BY col [ASC\|DESC]] [LIMIT n]`; `SELECT COUNT(*) FROM db.table ...`; `SELECT col, COUNT(*) ... GROUP BY col` (or `table` with `QueryExecutionContext.Database`) |
| Catalog | Resolves tables from Glue (`StorageDescriptor.Location`, columns, SerDe/InputFormat for CSV vs JSON) |
| Trail-shaped JSON | Top-level `{"Records":[...]}` objects expand one row per record; `.gz` objects decompress before parse |
| Results | In-memory result set. Optional `ResultConfiguration.OutputLocation` writes CSV under lab S3; write failures mark the query `FAILED` |
| WorkGroup | Optional. Defaults to `primary` |

Unsupported SQL fails with `InvalidRequestException` or a `FAILED` query execution (not an empty success). Missing Glue tables fail with `TABLE_NOT_FOUND`. Missing S3 location buckets fail closed with `FAILED` (not empty SUCCEEDED). If a listed object under the table prefix fails `GetObject`, the query is `FAILED` (no silent skip). An empty prefix listing that succeeds may return header-only `SUCCEEDED` (intentional when no objects match).

Lab JOIN is INNER only (aliases required on both sides; `ON` equality of `alias.col = alias.col`). `GROUP BY` supports a single column with `COUNT(*)` (optional group key in the SELECT list). `ORDER BY` supports a single column with optional `ASC`/`DESC`.

### Authz notes

Identity `EvaluateFull` on `athena:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws s3 mb s3://athena-lab --endpoint-url "$EP"
echo -e 'id,name\n1,alice\n2,bob' | aws s3 cp - s3://athena-lab/data/t1.csv --endpoint-url "$EP"
echo -e 'order_id,person_id,amount\n10,1,100\n11,1,50\n12,2,75' | aws s3 cp - s3://athena-lab/orders/o1.csv --endpoint-url "$EP"

aws glue create-database --database-input Name=labdb --endpoint-url "$EP"
aws glue create-table --database-name labdb --table-input '{
  "Name":"people",
  "StorageDescriptor":{
    "Location":"s3://athena-lab/data/",
    "Columns":[{"Name":"id","Type":"string"},{"Name":"name","Type":"string"}],
    "SerdeInfo":{"SerializationLibrary":"org.apache.hadoop.hive.serde2.lazy.LazySimpleSerDe","Parameters":{"field.delim":","}}
  }
}' --endpoint-url "$EP"
aws glue create-table --database-name labdb --table-input '{
  "Name":"orders",
  "StorageDescriptor":{
    "Location":"s3://athena-lab/orders/",
    "Columns":[{"Name":"order_id","Type":"string"},{"Name":"person_id","Type":"string"},{"Name":"amount","Type":"string"}],
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

# WHERE equality
QID=$(aws athena start-query-execution \
  --query-string "SELECT id, name FROM labdb.people WHERE name = 'alice'" \
  --query-execution-context Database=labdb \
  --endpoint-url "$EP" --query QueryExecutionId --output text)
aws athena get-query-results --query-execution-id "$QID" --endpoint-url "$EP"

# WHERE IN
QID=$(aws athena start-query-execution \
  --query-string "SELECT id, name FROM labdb.people WHERE name IN ('alice', 'bob')" \
  --query-execution-context Database=labdb \
  --endpoint-url "$EP" --query QueryExecutionId --output text)
aws athena get-query-results --query-execution-id "$QID" --endpoint-url "$EP"

# WHERE !=
QID=$(aws athena start-query-execution \
  --query-string "SELECT id, name FROM labdb.people WHERE name != 'bob'" \
  --query-execution-context Database=labdb \
  --endpoint-url "$EP" --query QueryExecutionId --output text)
aws athena get-query-results --query-execution-id "$QID" --endpoint-url "$EP"

# COUNT(*)
QID=$(aws athena start-query-execution \
  --query-string "SELECT COUNT(*) FROM labdb.people WHERE name = 'bob'" \
  --query-execution-context Database=labdb \
  --endpoint-url "$EP" --query QueryExecutionId --output text)
aws athena get-query-results --query-execution-id "$QID" --endpoint-url "$EP"

# INNER JOIN + ORDER BY
QID=$(aws athena start-query-execution \
  --query-string "SELECT p.name, o.order_id FROM labdb.people p JOIN labdb.orders o ON p.id = o.person_id WHERE p.name = 'alice' ORDER BY o.order_id ASC" \
  --query-execution-context Database=labdb \
  --endpoint-url "$EP" --query QueryExecutionId --output text)
aws athena get-query-results --query-execution-id "$QID" --endpoint-url "$EP"

# GROUP BY + COUNT(*)
QID=$(aws athena start-query-execution \
  --query-string "SELECT person_id, COUNT(*) FROM labdb.orders GROUP BY person_id ORDER BY person_id ASC" \
  --query-execution-context Database=labdb \
  --endpoint-url "$EP" --query QueryExecutionId --output text)
aws athena get-query-results --query-execution-id "$QID" --endpoint-url "$EP"

# ORDER BY DESC + LIMIT
QID=$(aws athena start-query-execution \
  --query-string "SELECT id, name FROM labdb.people ORDER BY name DESC LIMIT 1" \
  --query-execution-context Database=labdb \
  --endpoint-url "$EP" --query QueryExecutionId --output text)
aws athena get-query-results --query-execution-id "$QID" --endpoint-url "$EP"
```

Skip live Compose smoke when Docker is unavailable. Store and server unit tests cover the lab path without DinD.

## Not yet / deferred

- Broader SQL (`LEFT`/`RIGHT` joins, multi-column `GROUP BY`/`ORDER BY`, range inequalities (`<`/`>`/`BETWEEN`), `NOT IN`, subqueries, aggregates beyond `COUNT(*)`), CTAS, UNLOAD, INSERT, federated catalogs
- WorkGroup configuration matrix and result reuse
- Nested Trino / Presto / Spark engines
- Managed query results encryption options
