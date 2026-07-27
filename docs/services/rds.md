# Amazon RDS

**Status:** shipped (lab core)

Create, describe, and delete DB instances for engines `postgres`, `mysql`, and `mariadb`. Nested engines run inside DinD via the shared data-plane helper when `NOCTAXRIS_DOCKER_HOST` is set. The endpoint string is nested-network only. Compose does not publish DB ports on the host. Prefer RDS Data API on `:4566` for Postgres SQL; MySQL/MariaDB use the nested wire protocol only (no Data API).

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateDBInstance`, `DescribeDBInstances`, `DeleteDBInstance` |
| Engines | `postgres` (`postgres:16-alpine`, port 5432), `mysql` (`mysql:8.0`, port 3306), `mariadb` (`mariadb:11`, port 3306) |
| Secrets | Master user stored in Secrets Manager (`noctaxris/rds/<id>` JSON username/password) |
| Nested | Engine image on Internal network `noctaxris-data` when DinD is up |

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

aws rds create-db-instance \
  --db-instance-identifier labmysql1 \
  --db-instance-class db.t3.micro \
  --engine mysql \
  --master-username root \
  --master-user-password 'lab-password-1' \
  --allocated-storage 20 \
  --endpoint-url "$EP"

aws rds create-db-instance \
  --db-instance-identifier labmaria1 \
  --db-instance-class db.t3.micro \
  --engine mariadb \
  --master-username root \
  --master-user-password 'lab-password-1' \
  --allocated-storage 20 \
  --endpoint-url "$EP"

aws rds describe-db-instances \
  --db-instance-identifier labmysql1 \
  --endpoint-url "$EP"
```

Without DinD, status stays `creating` and the nested endpoint hostname is still recorded. With DinD, status becomes `available` after the nested container starts. Nested endpoints are not published on the host.

## Not yet / deferred

- RDS Data API for MySQL/MariaDB (Postgres only today; see [rds-data.md](rds-data.md))

## Out of lab scope

- Multi-AZ, read replicas, Aurora full cluster matrix (out of lab scope)
- IAM DB auth tokens (out of lab scope)
- Oracle / SQL Server engines (out of lab scope)

## Will not ship

- Host-published DB ports (will not ship; would break loopback-only `:4566` publish; use RDS Data API on `:4566` for Postgres, or an opt-in nested port overlay when documented)
