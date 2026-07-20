# EMR

**Status:** shipped (lab core, control-plane stub)

Cluster CRUD lite via `RunJobFlow`, `DescribeCluster`, `ListClusters`, and `TerminateJobFlows`. Returns control-plane status only. No host Spark or Hadoop install.

## Implemented

| Area | Actions |
|------|---------|
| Create | `RunJobFlow` (cluster enters `WAITING`) |
| Read | `DescribeCluster`, `ListClusters` |
| Terminate | `TerminateJobFlows` |
| Authz | Identity `EvaluateFull` on `elasticmapreduce:*` |

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws emr create-cluster \
  --name "noctaxris-emr-$RANDOM" \
  --release-label emr-7.0.0 \
  --instance-type m5.xlarge \
  --instance-count 1 \
  --endpoint-url "$EP"

aws emr list-clusters --endpoint-url "$EP"
```

`create-cluster` maps to `RunJobFlow` in the AWS CLI. Describe shows `WAITING` until TerminateJobFlows.

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Full step / bootstrap / instance-group matrix
- Nested Spark/Hadoop engines
- EMR Serverless and Studio
