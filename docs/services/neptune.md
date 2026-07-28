# Neptune

**Status:** shipped (lab core; nested Gremlin Server by default when DinD is up; optional Neo4j)

Create, describe, and delete Neptune clusters (`Engine=neptune`). Nested DinD defaults to `tinkerpop/gremlin-server:3.7.3` on Internal `noctaxris-data` (Gremlin WebSocket on `:8182`). Opt in to nested `neo4j:5-community` for Bolt/openCypher on `:7687`. Endpoint is nested-network only. Not DocumentDB. No host publish of Gremlin or Bolt ports by default.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateDBCluster`, `DescribeDBClusters`, `DeleteDBCluster` |
| Engine | `neptune` only (`docdb` rejected on this surface) |
| Nested backend | Default Gremlin Server; opt-in Neo4j (`NOCTAXRIS_NEPTUNE_ENGINE`, `GraphEngine` / `DbType`, or tag `noctaxris:neptune-engine`) |
| Status | `creating` until nested engine starts; `available` only after nested start; `failed` on DinD start/wait error |
| Endpoint | Gremlin: `{id}.neptune.noctaxris.internal:8182`; Neo4j Bolt: `{id}.neptune.noctaxris.internal:7687` (nested network only) |

### Authz notes

Identity `EvaluateFull` on `rds:CreateDBCluster`, `rds:DescribeDBClusters`, and `rds:DeleteDBCluster` (Neptune shares the RDS control-plane IAM action prefix). Sign requests as SigV4 service `neptune`. DocumentDB remains on service `rds` / `docdb` with `Engine=docdb` and continues to reject `Engine=neptune`.

### Nested graph backend selection

AWS `Engine` stays `neptune`. The nested graph process is selected separately (first match wins):

1. Tag `noctaxris:neptune-engine` or `noctaxris:graph-engine` (`gremlin` or `neo4j`)
2. Query parameter `GraphEngine` or `DbType`
3. Env `NOCTAXRIS_NEPTUNE_ENGINE` (`gremlin` default when unset/empty; `neo4j` opt-in)
4. Default: `gremlin`

Aliases: `tinkerpop` → gremlin; `opencypher` / `cypher` / `bolt` → neo4j. Unknown values return `InvalidParameterValue` (fail closed). Neo4j containers set `NEO4J_AUTH=none` so nested peers can speak Bolt without brokered passwords (Neptune authenticates at the AWS edge).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine` for nested Gremlin or Neo4j.

Default Gremlin:

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

Opt-in Neo4j (env):

```bash
# On the Noctaxris process / Compose service
export NOCTAXRIS_NEPTUNE_ENGINE=neo4j

aws neptune create-db-cluster \
  --db-cluster-identifier "noctaxris-neo4j-$RANDOM" \
  --engine neptune \
  --endpoint-url "$EP"
```

Opt-in Neo4j (per cluster via CLI tags):

```bash
aws neptune create-db-cluster \
  --db-cluster-identifier "noctaxris-neo4j-$RANDOM" \
  --engine neptune \
  --tags Key=noctaxris:neptune-engine,Value=neo4j \
  --endpoint-url "$EP"
```

Create starts as `creating`. Status becomes `available` only after a nested container starts on the DinD data network (no host publish). Without `noctaxris-engine`, the cluster stays `creating` (same honesty as DocumentDB/ElastiCache). With DinD, start or health-wait failures mark `failed`. Skip live Gremlin/Bolt smoke from the operator host (nested-network only). Unit tests cover control-plane CRUD and engine selection without Docker.

Gremlin from a nested peer on `noctaxris-data` (not the operator host):

```text
ws://{id}.neptune.noctaxris.internal:8182/gremlin
```

Bolt / openCypher from a nested peer when Neo4j is selected:

```text
bolt://{id}.neptune.noctaxris.internal:7687
```

### Operator loopback (opt-in)

Default Compose does not publish `:8182` or `:7687` on the host. For loopback Gremlin or Bolt from the operator machine, use `NOCTAXRIS_NESTED_PORT_PUBLISH=1` with `docker/compose.lab-nested-ports.yaml` (engine PortBindings plus `127.0.0.1` maps on `noctaxris-engine`). Fixed host ports are one-instance-per-port.

## Not yet / deferred

- CreateDBInstance / instance matrix
- IAM database authentication for Gremlin / Bolt
- HTTP Gremlin facade on `:4566`
- Host or WAN publish of Gremlin/Bolt ports (forbidden as default; loopback only via nested-ports overlay)
