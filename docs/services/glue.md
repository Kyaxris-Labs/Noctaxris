# Glue

**Status:** shipped (lab core)

Data Catalog database and table CRUD over sqlite. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Database | `CreateDatabase`, `GetDatabase`, `GetDatabases`, `DeleteDatabase` |
| Table | `CreateTable`, `GetTable`, `GetTables`, `DeleteTable` |

Tables store a storage location and a column list under `StorageDescriptor`.

### Authz notes

Identity `EvaluateFull` on `glue:*`. No Lake Formation or resource policy path.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws glue create-database --database-input Name=labdb --endpoint-url "$EP"
aws glue create-table --database-name labdb --table-input '{
  "Name":"t1",
  "StorageDescriptor":{"Location":"s3://lab/p","Columns":[{"Name":"id","Type":"string"}]}
}' --endpoint-url "$EP"
aws glue get-table --database-name labdb --name t1 --endpoint-url "$EP"
aws glue get-tables --database-name labdb --endpoint-url "$EP"
```

## Not yet / deferred

- Crawlers, ETL jobs, Lake Formation
- Athena query engine
