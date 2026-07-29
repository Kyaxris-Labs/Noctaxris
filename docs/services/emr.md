# EMR

**Status:** shipped (lab core, control-plane stub)

Cluster CRUD lite via `RunJobFlow`, `DescribeCluster`, `ListClusters`, and `TerminateJobFlows`. Job-flow steps via `AddJobFlowSteps`, `DescribeStep`, `ListSteps`, and `CancelSteps` (SQLite-backed; steps complete immediately with no Spark/Hadoop execution; CancelSteps marks steps `CANCELLED`). Instance groups/fleets metadata from `RunJobFlow`, cluster tags, and security configurations are persisted as lab state only.

## Implemented

| Area | Actions |
|------|---------|
| Create | `RunJobFlow` (cluster enters `WAITING`; persists `Instances.InstanceGroups` / `InstanceFleets` and `Tags` when provided) |
| Read | `DescribeCluster` (includes tags), `ListClusters` |
| Steps | `AddJobFlowSteps`, `DescribeStep`, `ListSteps` (steps persist as `COMPLETED`); `CancelSteps` (marks steps `CANCELLED`) |
| Instance collection | `ListInstanceGroups`, `ListInstanceFleets` |
| Tags | `AddTags`, `RemoveTags` (cluster `ResourceId`) |
| Security configs | `CreateSecurityConfiguration`, `DescribeSecurityConfiguration`, `DeleteSecurityConfiguration`, `ListSecurityConfigurations` (stored JSON) |
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
  --tags Key=env,Value=lab \
  --endpoint-url "$EP"

CID=$(aws emr list-clusters --active --query 'Clusters[0].Id' --output text --endpoint-url "$EP")

aws emr list-instance-groups --cluster-id "$CID" --endpoint-url "$EP"

aws emr add-steps --cluster-id "$CID" --steps Type=CUSTOM_JAR,Name=smoke,Jar=command-runner.jar,Args=echo,ok \
  --endpoint-url "$EP"

SID=$(aws emr list-steps --cluster-id "$CID" --query 'Steps[0].Id' --output text --endpoint-url "$EP")
aws emr cancel-steps --cluster-id "$CID" --step-ids "$SID" --endpoint-url "$EP"

aws emr create-security-configuration --name lab-sec \
  --security-configuration '{"EncryptionConfiguration":{}}' --endpoint-url "$EP"
aws emr describe-security-configuration --name lab-sec --endpoint-url "$EP"
```

`create-cluster` maps to `RunJobFlow` in the AWS CLI. Describe shows `WAITING` until TerminateJobFlows. Instance groups from create are listable; CancelSteps flips step state to `CANCELLED` in the lab stub.

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Nested Spark/Hadoop engines
- EMR Serverless and Studio
- `AddInstanceGroups` / `AddInstanceFleet` / `ListInstances` (mutate after create)
