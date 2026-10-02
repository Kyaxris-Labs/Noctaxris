# ElastiCache

**Status:** shipped (lab core, control plane)

Create, describe, and delete Valkey or Redis OSS cache clusters. Endpoint is a nested-network hostname and port only. No host publish of Redis ports. No Redis client dependency in the emulator process.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateCacheCluster`, `DescribeCacheClusters`, `DeleteCacheCluster` |
| Engines | `redis`, `valkey` |
| Endpoint | `{id}.cache.noctaxris.internal:6379` (nested network, not WAN or host published) |
| Wire AUTH | Default: no AUTH on nested Valkey. Opt-in: `NOCTAXRIS_REDIS_AUTH=1` starts with `--requirepass noctaxris-cache-lab` (same value as the Secrets Manager master secret) |

### Authz notes

Identity `EvaluateFull` on `elasticache:*`.

### Nested wire

Default Compose keeps Redis on Internal `noctaxris-data` only. Without AUTH, any peer on that network can speak RESP. That is Medium residual inside DinD. `NOCTAXRIS_NESTED_PORT_PUBLISH=1` with `compose.lab-nested-ports.yaml` publishes loopback `:6379` on the engine hop and raises exposure (treat as High if the host is shared). Prefer leaving AUTH off only on single-tenant lab hosts, or set `NOCTAXRIS_REDIS_AUTH=1`.

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

- Redis ACL users / IAM auth token full matrix (opt-in is single `--requirepass` only)
- Cluster mode enabled / replication groups full matrix
- Encryption at rest
- Host or WAN publish of cache ports (forbidden)
