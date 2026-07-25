# Glue

**Status:** shipped (lab core)

Data Catalog database and table CRUD over sqlite, plus a synchronous S3 crawler lite. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Database | `CreateDatabase`, `GetDatabase`, `GetDatabases`, `DeleteDatabase` |
| Table | `CreateTable`, `GetTable`, `GetTables`, `DeleteTable` |
| Crawler | `CreateCrawler`, `StartCrawler`, `GetCrawler`, `DeleteCrawler`, `ListCrawlers` |

Tables store a storage location, column list, partition keys, InputFormat/OutputFormat, and SerDeInfo under `StorageDescriptor` (Athena prerequisites).

### Crawler lite

`StartCrawler` runs synchronously in-process (no Spark). For each `S3Targets[].Path`, the crawler lists object keys under the prefix, infers CSV vs JSON from the key suffix or leading bytes, and creates or updates a catalog table from the header row (CSV) or first JSON object keys. Crawler state transitions `READY` → `RUNNING` → `READY`. Missing S3 buckets fail closed. When `Role` is set on create, `iam:PassRole` is enforced with trust for `glue.amazonaws.com`.

### Authz notes

Identity `EvaluateFull` on `glue:*`. No Lake Formation or resource policy path.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws glue create-database --database-input Name=labdb --endpoint-url "$EP"
aws s3 mb s3://crawl-lab --endpoint-url "$EP"
printf 'id,name\n1,alice\n' | aws s3 cp - s3://crawl-lab/data/people.csv --endpoint-url "$EP"
aws glue create-crawler --name csv-crawler --role "" --database-name labdb \
  --targets '{"S3Targets":[{"Path":"s3://crawl-lab/data/"}]}' --endpoint-url "$EP"
aws glue start-crawler --name csv-crawler --endpoint-url "$EP"
aws glue get-crawler --name csv-crawler --endpoint-url "$EP"
aws glue get-table --database-name labdb --name people --endpoint-url "$EP"
aws glue create-table --database-name labdb --table-input '{
  "Name":"t1",
  "StorageDescriptor":{
    "Location":"s3://lab/p",
    "Columns":[{"Name":"id","Type":"string"}],
    "InputFormat":"org.apache.hadoop.mapred.TextInputFormat",
    "OutputFormat":"org.apache.hadoop.hive.ql.io.HiveIgnoreKeyTextOutputFormat",
    "SerdeInfo":{"SerializationLibrary":"org.apache.hadoop.hive.serde2.lazy.LazySimpleSerDe","Parameters":{"field.delim":","}}
  },
  "PartitionKeys":[{"Name":"dt","Type":"string"}]
}' --endpoint-url "$EP"
aws glue get-table --database-name labdb --name t1 --endpoint-url "$EP"
aws glue get-tables --database-name labdb --endpoint-url "$EP"
```

Athena query smoke: [athena.md](athena.md).

## Not yet / deferred

- ETL jobs, Lake Formation
- Partition value registration and MSCK REPAIR
- Asynchronous distributed crawls (lab crawler is single-process sync only)
