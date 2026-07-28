# MSK (Managed Streaming for Kafka)

**Status:** shipped (lab core; nested Redpanda when DinD is up)

Cluster CRUD and `GetBootstrapBrokers` for Amazon MSK. Nested DinD uses `redpandadata/redpanda:v24.2.4` on Internal `noctaxris-data` (Kafka API on `:9092`). Bootstrap brokers are nested-network addresses only. No host publish of broker ports unless you opt in (see below).

## Modes

| Mode | Env | CreateCluster | Bootstrap (`ACTIVE`) | Second cluster | DeleteCluster |
|------|-----|---------------|------------------------|----------------|---------------|
| Per-cluster (default) | `NOCTAXRIS_SHARED_KAFKA` off | Starts `noctaxris-msk-<name>` in DinD | `noctaxris-msk-<name>:9092` | Allowed (separate nested containers) | Stops/removes that nested container + metadata |
| Shared singleton | `NOCTAXRIS_SHARED_KAFKA=1` | Binds metadata to `noctaxris-lab-kafka` (idempotent ensure) | `noctaxris-lab-kafka:9092` | Denied (see errors) | Metadata only; **does not** stop `noctaxris-lab-kafka` |
| No DinD | `NOCTAXRIS_DOCKER_HOST` empty | Row may reach `FAILED` | empty | Per-mode rules | Metadata cleanup |

Shared Kafka allows **at most one CREATING or ACTIVE** MSK metadata row per Noctaxris process (all accounts). `FAILED` and `DELETING` rows do not hold that slot, so a failed create can be retried without an explicit `DeleteCluster` of the failed row. Topic/ACL isolation across accounts is not implemented; treat shared Kafka as a single lab broker, not multi-tenant.

Enabling shared Kafka while per-cluster `noctaxris-msk-*` containers or matching metadata still exist fails closed (no silent remap to the singleton). Delete those clusters with shared mode off, or clean up DinD manually, before turning shared Kafka on.

### Pinned errors (shared mode)

| `__type` | Message (exact) | When |
|----------|-----------------|------|
| `LimitExceededException` | `Shared Kafka allows only one cluster per Noctaxris process` | Second `CreateCluster` while a CREATING or ACTIVE MSK row exists |
| `BadRequestException` | `Delete existing per-cluster MSK containers before enabling shared Kafka binds` | Shared on + leftover per-cluster MSK binds |

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateCluster`, `DescribeCluster`, `ListClusters`, `DeleteCluster` |
| Brokers | `GetBootstrapBrokers` (nested address when `ACTIVE`) |
| Protocol | REST-JSON (`/v1/clusters`) and `X-Amz-Target` `Kafka_1.0.*` |
| Status | `CREATING` → `ACTIVE` when the nested container is healthy; `FAILED` when DinD is unset or start/wait fails |

### Authz notes

Identity `EvaluateFull` on `kafka:*`. Sign as SigV4 service `kafka`.

## Opt-in shared Kafka (Compose)

Default `docker/compose.yaml` leaves shared Kafka off. To start the singleton Redpanda and narrow engine PortBindings for API-side `:9092` dial:

```bash
docker compose -f docker/compose.yaml -f docker/compose.lab-brokers.yaml --env-file docker/.env up --build
```

Operator loopback `127.0.0.1:9092` additionally needs `docker/compose.lab-nested-ports.yaml` (engine hop). Nested clients inside DinD always use `noctaxris-lab-kafka:9092`, not host loopback.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine` for nested Redpanda.

```bash
aws kafka create-cluster \
  --cluster-name "noctaxris-msk-$RANDOM" \
  --kafka-version "3.6.0" \
  --number-of-broker-nodes 1 \
  --broker-node-group-info '{"InstanceType":"kafka.m5.large","ClientSubnets":["subnet-1"]}' \
  --endpoint-url "$EP"

aws kafka list-clusters --endpoint-url "$EP"

CLUSTER_ARN=$(aws kafka list-clusters --query 'ClusterInfoList[0].ClusterArn' --output text --endpoint-url "$EP")
aws kafka describe-cluster --cluster-arn "$CLUSTER_ARN" --endpoint-url "$EP"
aws kafka get-bootstrap-brokers --cluster-arn "$CLUSTER_ARN" --endpoint-url "$EP"

aws kafka delete-cluster --cluster-arn "$CLUSTER_ARN" --endpoint-url "$EP"
```

With DinD healthy, DescribeCluster shows `ACTIVE` and GetBootstrapBrokers returns the nested bootstrap for your mode (`noctaxris-msk-<name>:9092` or `noctaxris-lab-kafka:9092` with shared Kafka on). Reachable only from other nested containers on `noctaxris-data`, not from the operator host unless the nested-ports overlay is active. Without DinD, state is `FAILED` and bootstrap string is empty. Skip live Kafka producer/consumer smoke from the operator host.

## Not yet / deferred

- CreateClusterV2 / serverless shapes
- TLS / SASL / **IAM** auth broker endpoints (control plane only; no full MSK IAM wire auth)
- Multi-broker topology and ZooKeeper strings
- Host or WAN publish of Kafka ports (forbidden by default; loopback opt-in only)
