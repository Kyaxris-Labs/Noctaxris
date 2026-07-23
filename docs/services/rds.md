# Amazon RDS

**Status:** shipped (lab core)

Create, describe, and delete Postgres DB instances. Nested Postgres runs inside DinD via the shared data-plane helper when `NOCTAXRIS_DOCKER_HOST` is set. The endpoint string is nested-network only. Compose does not publish Postgres ports on the host. Prefer RDS Data API on `:4566` for SQL.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateDBInstance`, `DescribeDBInstances`, `DeleteDBInstance` |
| Engine | `postgres` (lab core) |
| Secrets | Master user stored in Secrets Manager (`noctaxris/rds/<id>` JSON username/password) |
| Nested | `postgres:16-alpine` on Internal network `noctaxris-data` when DinD is up |

### Authz notes

Identity `EvaluateFull` on `rds:CreateDBInstance`, `rds:DescribeDBInstances`, `rds:DeleteDBInstance`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws rds create-db-instance \
  --db-instance-identifier labpg1 \
  --db-instance-class db.t3.micro \
  --engine postgres \
  --master-username postgres \
  --master-user-password 'lab-password-1' \
  --allocated-storage 20 \
  --endpoint-url "$EP"

aws rds describe-db-instances \
  --db-instance-identifier labpg1 \
  --endpoint-url "$EP"
```

Without DinD, status stays `creating` and the nested endpoint is still recorded. With DinD, status becomes `available` after the nested container starts.

## Out of lab scope

- MySQL engine image (out of lab scope; Postgres nested DinD + Data API is the marketed data path)
- Multi-AZ, read replicas, Aurora full cluster matrix (out of lab scope)
- IAM DB auth tokens (out of lab scope)

## Will not ship

- Host-published Postgres ports (will not ship; would break loopback-only `:4566` publish; use RDS Data API on `:4566`)
