# OpenSearch

**Status:** shipped (lab core, control-plane stub)

Domain CRUD for Amazon OpenSearch Service. Returns a loopback-only stub endpoint (`stub://127.0.0.1/opensearch/...`). No nested OpenSearch process and no WAN-published search ports.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateDomain`, `DescribeDomain`, `ListDomainNames`, `DeleteDomain` |
| Endpoint | Stub `Endpoint` on `127.0.0.1` (not a live listener) |
| Engine | `EngineVersion` string stored (default `OpenSearch_2.11`) |

### Authz notes

Identity `EvaluateFull` on `es:*` (OpenSearch Service IAM prefix).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws opensearch create-domain \
  --domain-name "noctaxris-os-$RANDOM" \
  --engine-version OpenSearch_2.11 \
  --endpoint-url "$EP"

aws opensearch list-domain-names --endpoint-url "$EP"
aws opensearch describe-domain --domain-name noctaxris-os-example --endpoint-url "$EP"
```

DescribeDomain shows the stub endpoint. Do not expect a live HTTPS OpenSearch listener on that URL.

## Not yet / deferred

- Nested DinD OpenSearch cluster
- Full query DSL proxy and fine-grained access control
- VPC options, custom endpoints, and Autotune parity
