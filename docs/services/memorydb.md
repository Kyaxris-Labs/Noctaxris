# MemoryDB

**Status:** shipped (lab core, control plane)

Create, describe, and delete Valkey or Redis MemoryDB clusters. Endpoint is a nested-network hostname and port only. No host publish of Redis ports. Users and ACLs are empty stubs (control-plane theatre only; no wire ACL enforcement).

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateCluster`, `DescribeClusters`, `DeleteCluster` |
| Stubs | `DescribeUsers`, `DescribeACLs` (empty lists; not multi-tenant ACL claims) |
| Engines | `redis`, `valkey` |
| Protocol | JSON 1.1 (`X-Amz-Target: AmazonMemoryDB.*`) |
| Endpoint | `{name}.memorydb.noctaxris.internal:6379` (nested network, not WAN or host published) |
| Wire AUTH | Default: no AUTH on nested Valkey. Opt-in: `NOCTAXRIS_REDIS_AUTH=1` starts with `--requirepass noctaxris-memorydb-lab` (same value as the Secrets Manager master secret). `ACLName` (default `open-access`) is metadata only |

### Authz notes

Identity `EvaluateFull` on `memorydb:*`.

### Nested wire

Same posture as ElastiCache: nested-Internal no-AUTH is Medium among DinD peers; nested-ports loopback publish raises it. Do not treat empty `DescribeUsers` / `DescribeACLs` as real isolation.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws memorydb create-cluster \
  --cluster-name "noctaxris-memdb-$RANDOM" \
  --node-type db.t4g.small \
  --acl-name open-access \
  --engine redis \
  --endpoint-url "$EP"

aws memorydb describe-clusters \
  --cluster-name "noctaxris-memdb-..." \
  --endpoint-url "$EP"
```

Describe returns the nested `ClusterEndpoint`. Create starts as `creating`. Status becomes `available` only after a nested Valkey or Redis container starts on the DinD data network (no host publish). Without `noctaxris-engine`, the cluster stays `creating` (same honesty as ElastiCache). Skip live RESP smoke when Docker is unavailable. Unit tests cover control-plane CRUD without Docker.

## Not yet / deferred

- CreateUser / DeleteUser / CreateACL / DeleteACL (Describe returns empty; wire has no ACL)
- TLS / IAM auth token full matrix (opt-in is single `--requirepass` only)
- Multi-shard / Multi-AZ depth
- Host or WAN publish of MemoryDB ports (forbidden)
