# ElastiCache

**Status:** shipped (lab core, control plane)

Create, describe, and delete Valkey or Redis OSS cache clusters. Endpoint is a nested-network hostname and port only. No host publish of Redis ports. No Redis client dependency in the emulator process.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateCacheCluster`, `DescribeCacheClusters`, `DeleteCacheCluster` |
| Engines | `redis`, `valkey` |
| Endpoint | `{id}.cache.noctaxris.internal:6379` (nested network, not WAN or host published) |

### Authz notes

Identity `EvaluateFull` on `elasticache:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws elasticache create-cache-cluster \
  --cache-cluster-id "noctaxris-cache-$RANDOM" \
  --engine redis \
  --cache-node-type cache.t3.micro \
  --num-cache-nodes 1 \
  --endpoint-url "$EP"

aws elasticache describe-cache-clusters \
  --cache-cluster-id "noctaxris-cache-..." \
  --show-cache-node-info \
  --endpoint-url "$EP"
```

Describe returns the nested endpoint. Create starts as `creating`. Status becomes `available` only after a nested Valkey or Redis container starts on the DinD data network (no host publish). Without `noctaxris-engine`, the cluster stays `creating` (same honesty as RDS). Skip live RESP smoke when Docker is unavailable. Unit tests cover control-plane CRUD without Docker.

## Not yet / deferred

- Redis AUTH / IAM auth token full matrix (nested lab image has no AUTH by default)
- Cluster mode enabled / replication groups full matrix
- Encryption at rest
- MemoryDB
- Host or WAN publish of cache ports (forbidden)
