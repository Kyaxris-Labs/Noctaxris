# MSK (Managed Streaming for Kafka)

**Status:** shipped (lab core; nested Redpanda when DinD is up)

Cluster CRUD and `GetBootstrapBrokers` for Amazon MSK. Nested DinD uses `redpandadata/redpanda:v24.2.4` on Internal `noctaxris-data` (Kafka API on `:9092`). Bootstrap brokers are nested-network addresses only. No host publish of broker ports.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateCluster`, `DescribeCluster`, `ListClusters`, `DeleteCluster` |
| Brokers | `GetBootstrapBrokers` (nested `noctaxris-msk-<name>:9092` when `ACTIVE`) |
| Protocol | REST-JSON (`/v1/clusters`) and `X-Amz-Target` `Kafka_1.0.*` |
| Status | `CREATING` → `ACTIVE` when the nested container is healthy; `FAILED` when DinD is unset or start/wait fails |

### Authz notes

Identity `EvaluateFull` on `kafka:*`. Sign as SigV4 service `kafka`.

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

With DinD healthy, DescribeCluster shows `ACTIVE` and GetBootstrapBrokers returns `noctaxris-msk-<name>:9092` (reachable only from other nested containers on `noctaxris-data`, not from the host). Without DinD, state is `FAILED` and bootstrap string is empty. Skip live Kafka producer/consumer smoke from the operator host.

## Not yet / deferred

- CreateClusterV2 / serverless shapes
- TLS / SASL / IAM auth broker endpoints
- Multi-broker topology and ZooKeeper strings
- Host or WAN publish of Kafka ports (forbidden)
