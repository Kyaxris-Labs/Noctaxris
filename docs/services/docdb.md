# DocumentDB

**Status:** shipped (lab core, control plane)

Create, describe, and delete DocumentDB clusters (`Engine=docdb`). Endpoint is a nested-network hostname and port only. Not Neptune. No host publish of Mongo ports.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateDBCluster`, `DescribeDBClusters`, `DeleteDBCluster` |
| Engine | `docdb` only (Neptune rejected) |
| Endpoint | `{id}.docdb.noctaxris.internal:27017` (nested network, not WAN or host published) |

### Authz notes

Identity `EvaluateFull` on `rds:CreateDBCluster`, `rds:DescribeDBClusters`, and `rds:DeleteDBCluster` (DocumentDB shares the RDS IAM action prefix).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws docdb create-db-cluster \
  --db-cluster-identifier "noctaxris-docdb-$RANDOM" \
  --engine docdb \
  --master-username labadmin \
  --master-user-password 'lab-password-1' \
  --endpoint-url "$EP"

aws docdb describe-db-clusters \
  --db-cluster-identifier "noctaxris-docdb-..." \
  --endpoint-url "$EP"
```

The AWS CLI DocumentDB client typically signs as service `rds`. Describe returns the nested endpoint. With `noctaxris-engine` up, create starts a nested Mongo-compatible image with init root credentials from the master secret. Skip live Mongo smoke when Docker is unavailable. Unit tests cover control-plane CRUD without Docker.

## Not yet / deferred

- Neptune (deferred product line)
- Change streams
- Full TLS client auth matrix
- Host or WAN publish of document DB ports (forbidden)
