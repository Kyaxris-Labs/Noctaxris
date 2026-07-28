# EMR

**Status:** shipped (lab core, control-plane stub)

Cluster CRUD lite via `RunJobFlow`, `DescribeCluster`, `ListClusters`, and `TerminateJobFlows`. Job-flow steps via `AddJobFlowSteps`, `DescribeStep`, and `ListSteps` (SQLite-backed; steps complete immediately with no Spark/Hadoop execution).

## Implemented

| Area | Actions |
|------|---------|
| Create | `RunJobFlow` (cluster enters `WAITING`) |
| Read | `DescribeCluster`, `ListClusters` |
| Steps | `AddJobFlowSteps`, `DescribeStep`, `ListSteps` (steps persist as `COMPLETED`) |
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

CID=$(aws emr list-clusters --active --query 'Clusters[0].Id' --output text --endpoint-url "$EP")

aws emr add-steps --cluster-id "$CID" --steps Type=CUSTOM_JAR,Name=smoke,Jar=command-runner.jar,Args=echo,ok \
  --endpoint-url "$EP"

aws emr list-steps --cluster-id "$CID" --endpoint-url "$EP"
```

`create-cluster` maps to `RunJobFlow` in the AWS CLI. Describe shows `WAITING` until TerminateJobFlows.

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- `CancelSteps`, instance groups/fleets, security configurations, tags
- Nested Spark/Hadoop engines
- EMR Serverless and Studio
