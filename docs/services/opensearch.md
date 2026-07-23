# OpenSearch

**Status:** shipped (lab core; nested OpenSearch when DinD is up)

Domain CRUD for Amazon OpenSearch Service. Nested DinD uses `opensearchproject/opensearch:2.11.1` on Internal `noctaxris-data`. Search ports are never published on the host.

| DinD | DomainStatus | Endpoint |
|------|--------------|----------|
| Healthy nested container | `Active` (`Created=true`) | Nested `noctaxris-opensearch-<name>:9200` |
| DinD unset / start or wait failure | `CreateFailed` (`Created=false`) | `stub://127.0.0.1/opensearch/...` |

Create inserts `Creating`, then the data-plane helper promotes or fail-closes. Never treat `Active` as available on `stub://`.

On `CreateFailed`, DescribeDomain / CreateDomain may include a lab `FailureReason` string when nested logs or wait errors match known bootstrap failures (`vm.max_map_count`, memory lock). Noctaxris does not apply host `sysctl` from the API container.

### Host `vm.max_map_count` (fail-closed)

Nested OpenSearch needs host `vm.max_map_count` of at least `262144` (OpenSearch bootstrap check). When the nested process cannot start for that reason (common on Docker Desktop / WSL2), CreateDomain stays **fail-closed**: `CreateFailed` with a `stub://` endpoint and, when detected, a `FailureReason` hint. There is no silent Active-on-stub promotion and no automatic host sysctl from inside Noctaxris.

This applies when you need an **Active** nested domain (query labs that rely on the nested engine). Control-plane-only labs that only assert `CreateFailed` / `stub://` do not need the sysctl.

| Host | Steps |
|------|--------|
| Linux (bare metal / VM) | Check: `cat /proc/sys/vm/max_map_count`. Raise for the current boot: `sudo sysctl -w vm.max_map_count=262144`. Persist: add `vm.max_map_count=262144` under `/etc/sysctl.d/` (or `/etc/sysctl.conf`) and run `sudo sysctl --system` |
| Docker Desktop on Windows (WSL2) | Raise inside the **Desktop/WSL VM**, not only on Windows. Session: `wsl -d docker-desktop sysctl -w vm.max_map_count=262144` (or enter `wsl -d docker-desktop` then run `sysctl -w vm.max_map_count=262144`). Persist across Desktop restarts via `%USERPROFILE%\.wslconfig` with `[wsl2]` / `kernelCommandLine = "sysctl.vm.max_map_count=262144"`, then `wsl --shutdown` and restart Docker Desktop |
| Docker Desktop on macOS | Raise `vm.max_map_count` inside the Desktop Linux VM (same requirement as Linux). If you cannot change it, expect `CreateFailed` |

After raising the value, delete any `CreateFailed` domain and CreateDomain again. If you cannot change the VM sysctl, keep using labs that do not require nested search.

See also [ops.md](../ops.md) for Compose overlays and nested smoke.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateDomain`, `DescribeDomain`, `ListDomainNames`, `DeleteDomain` |
| Status | `Creating` → `Active` or `CreateFailed` |
| Failure hint | Lab `FailureReason` on CreateFailed when mmap / memory-lock / nested wait evidence is classified |
| Network | Internal nested network only; no host publish of 9200/443 |
| Engine | `EngineVersion` string stored (default `OpenSearch_2.11`) |

### Authz notes

Identity `EvaluateFull` on `es:*` (OpenSearch Service IAM prefix).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine` for nested OpenSearch. Raise `vm.max_map_count` on the DinD host first if you expect `Active`.

```bash
aws opensearch create-domain \
  --domain-name "noctaxris-os-$RANDOM" \
  --engine-version OpenSearch_2.11 \
  --endpoint-url "$EP"

aws opensearch list-domain-names --endpoint-url "$EP"
aws opensearch describe-domain --domain-name noctaxris-os-example --endpoint-url "$EP"
```

With DinD healthy, adequate `vm.max_map_count`, and the nested process accepting connections, DescribeDomain shows `Active` and a nested host:port (reachable only from nested peers on `noctaxris-data`). Without DinD (or on start failure), status is `CreateFailed` with `stub://`; check `FailureReason` for mmap / memory-lock hints when present.

## Not yet / deferred

- Query-plane index/search subset and full query DSL proxy
- Fine-grained access control
- VPC options, custom endpoints, and Autotune parity
- Automatic host sysctl tuning for Desktop/WSL (operator must set `vm.max_map_count` when nested Active is required; fail-closed by design)
