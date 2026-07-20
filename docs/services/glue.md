# Glue

**Status:** shipped (lab core)

Data Catalog database and table CRUD over sqlite. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Database | `CreateDatabase`, `GetDatabase`, `GetDatabases`, `DeleteDatabase` |
| Table | `CreateTable`, `GetTable`, `GetTables`, `DeleteTable` |

Tables store a storage location, column list, partition keys, InputFormat/OutputFormat, and SerDeInfo under `StorageDescriptor` (Athena prerequisites).

### Authz notes

Identity `EvaluateFull` on `glue:*`. No Lake Formation or resource policy path.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws glue create-database --database-input Name=labdb --endpoint-url "$EP"
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

- Crawlers, ETL jobs, Lake Formation
- Partition value registration and MSCK REPAIR