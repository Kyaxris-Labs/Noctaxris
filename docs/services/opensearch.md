# OpenSearch

**Status:** shipped (lab core; nested OpenSearch when DinD is up)

Domain CRUD for Amazon OpenSearch Service. Nested DinD uses `opensearchproject/opensearch:2.11.1` on Internal `noctaxris-data`. Search ports are never published on the host.

| DinD | DomainStatus | Endpoint |
|------|--------------|----------|
| Healthy nested container | `Active` (`Created=true`) | Nested `noctaxris-opensearch-<name>:9200` |
| DinD unset / start or wait failure | `CreateFailed` (`Created=false`) | `stub://127.0.0.1/opensearch/...` |

Create inserts `Creating`, then the data-plane helper promotes or fail-closes. Never treat `Active` as available on `stub://`.

### Host `vm.max_map_count` (fail-closed)

Nested OpenSearch needs a high enough host `vm.max_map_count` (OpenSearch typically wants `262144`). When the nested process cannot start for that reason (common on Docker Desktop / WSL2), CreateDomain stays **fail-closed**: `CreateFailed` with a `stub://` endpoint. There is no silent Active-on-stub promotion.

| Host | Guidance |
|------|----------|
| Linux | `sysctl -w vm.max_map_count=262144` (persist via `/etc/sysctl.d/` if you run nested OpenSearch often) |
| Docker Desktop / WSL2 | Raise `vm.max_map_count` inside the Desktop/WSL VM (not only on the Windows host). If you cannot change it, expect `CreateFailed` and use labs that do not require nested search |

See also [ops.md](../ops.md) for Compose overlays and nested smoke.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateDomain`, `DescribeDomain`, `ListDomainNames`, `DeleteDomain` |
| Status | `Creating` → `Active` or `CreateFailed` |
| Network | Internal nested network only; no host publish of 9200/443 |
| Engine | `EngineVersion` string stored (default `OpenSearch_2.11`) |

### Authz notes

Identity `EvaluateFull` on `es:*` (OpenSearch Service IAM prefix).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine` for nested OpenSearch.

```bash
aws opensearch create-domain \
  --domain-name "noctaxris-os-$RANDOM" \
  --engine-version OpenSearch_2.11 \
  --endpoint-url "$EP"

aws opensearch list-domain-names --endpoint-url "$EP"
aws opensearch describe-domain --domain-name noctaxris-os-example --endpoint-url "$EP"
```

With DinD healthy and the nested process accepting connections, DescribeDomain shows `Active` and a nested host:port (reachable only from nested peers on `noctaxris-data`). Without DinD (or on start failure), status is `CreateFailed` with `stub://`.

## Not yet / deferred

- Query-plane index/search subset and full query DSL proxy
- Fine-grained access control
- VPC options, custom endpoints, and Autotune parity
- Automatic host sysctl tuning for Desktop/WSL (operator must set `vm.max_map_count` when nested Active is required)
