# Neptune

**Status:** shipped (lab core; nested Gremlin Server when DinD is up)

Create, describe, and delete Neptune clusters (`Engine=neptune`). Nested DinD uses `tinkerpop/gremlin-server:3.7.3` on Internal `noctaxris-data` (Gremlin WebSocket on `:8182`). Endpoint is nested-network only. Not DocumentDB. No host publish of Gremlin ports.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateDBCluster`, `DescribeDBClusters`, `DeleteDBCluster` |
| Engine | `neptune` only (`docdb` rejected on this surface) |
| Status | `creating` until nested Gremlin Server starts; `available` only after nested start; `failed` on DinD start/wait error |
| Endpoint | `{id}.neptune.noctaxris.internal:8182` (nested network, not WAN or host published) |

### Authz notes

Identity `EvaluateFull` on `rds:CreateDBCluster`, `rds:DescribeDBClusters`, and `rds:DeleteDBCluster` (Neptune shares the RDS control-plane IAM action prefix). Sign requests as SigV4 service `neptune`. DocumentDB remains on service `rds` / `docdb` with `Engine=docdb` and continues to reject `Engine=neptune`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine` for nested Gremlin.

```bash
aws neptune create-db-cluster \
  --db-cluster-identifier "noctaxris-neptune-$RANDOM" \
  --engine neptune \
  --endpoint-url "$EP"

aws neptune describe-db-clusters \
  --db-cluster-identifier "noctaxris-neptune-..." \
  --query 'DBClusters[0].{Endpoint:Endpoint,Port:Port,Status:Status}' \
  --endpoint-url "$EP"
```

Create starts as `creating`. Status becomes `available` only after a nested Gremlin Server container starts on the DinD data network (no host publish). Without `noctaxris-engine`, the cluster stays `creating` (same honesty as DocumentDB/ElastiCache). With DinD, start or health-wait failures mark `failed`. Skip live Gremlin smoke from the operator host (nested-network only). Unit tests cover control-plane CRUD without Docker.

Gremlin from a nested peer on `noctaxris-data` (not the operator host):

```text
ws://{id}.neptune.noctaxris.internal:8182/gremlin
```

## Not yet / deferred

- openCypher / Neo4j backend
- CreateDBInstance / instance matrix
- IAM database authentication for Gremlin
- HTTP Gremlin facade on `:4566`
- Host or WAN publish of Gremlin ports (forbidden)
