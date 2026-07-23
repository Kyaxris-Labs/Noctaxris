# Batch

**Status:** shipped

Lab AWS Batch core: compute environments, job queues, job definitions, and SubmitJob on nested DinD. REST JSON protocol on `/v1/*` paths (SigV4 service `batch`). Jobs reuse the nested Docker client (same TLS DinD host as Lambda and ECS). No host `docker.sock`.

## Implemented

| Area | Actions |
|------|---------|
| Compute environment | `CreateComputeEnvironment`, `DescribeComputeEnvironments` |
| Job queue | `CreateJobQueue`, `DescribeJobQueues` |
| Job definition | `RegisterJobDefinition`, `DescribeJobDefinitions` |
| Jobs | `SubmitJob`, `DescribeJobs` |
| Roles | Optional `serviceRole` on compute environment requires PassRole for `batch.amazonaws.com`. Optional `jobRoleArn` on container properties requires PassRole for `ecs-tasks.amazonaws.com`. When `jobRoleArn` is set, SubmitJob mints temporary AWS_* credentials for that role into the nested container (same pattern as ECS RunTask) |
| Compute | Nested containers via Compose `noctaxris-engine` (DinD TLS). SubmitJob reuses the nested ECS run helper on Internal network `noctaxris-ecs` (host-gateway ExtraHosts off by default). Lab registry image refs (`127.0.0.1:4566/...`) are rewritten and pulled with a registry token before start |

### Authz notes

Batch APIs use identity `EvaluateFull`.

PassRole is enforced when role ARNs are present. Job role trust follows the ECS tasks principal used by AWS Batch container jobs.

Without `NOCTAXRIS_DOCKER_HOST`, `SubmitJob` returns compute unavailable.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine`. Skip live SubmitJob when Docker is unavailable (unit tests cover PassRole and compute-unavailable).

```bash
BATCH_TRUST='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"batch.amazonaws.com"},"Action":"sts:AssumeRole"}]}'
JOB_TRUST='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}'

aws iam create-role --role-name LabBatchServiceRole --assume-role-policy-document "$BATCH_TRUST" --endpoint-url "$EP"
aws iam create-role --role-name LabBatchJobRole --assume-role-policy-document "$JOB_TRUST" --endpoint-url "$EP"
SVC=$(aws iam get-role --role-name LabBatchServiceRole --endpoint-url "$EP" --query Role.Arn --output text)
JOB=$(aws iam get-role --role-name LabBatchJobRole --endpoint-url "$EP" --query Role.Arn --output text)

aws batch create-compute-environment \
  --compute-environment-name lab-ce \
  --type MANAGED \
  --service-role "$SVC" \
  --endpoint-url "$EP"

aws batch create-job-queue \
  --job-queue-name lab-jq \
  --priority 1 \
  --compute-environment-order order=1,computeEnvironment=lab-ce \
  --endpoint-url "$EP"

aws batch register-job-definition \
  --job-definition-name lab-jd \
  --type container \
  --container-properties "{\"image\":\"alpine:3.20\",\"command\":[\"echo\",\"batch-ok\"],\"jobRoleArn\":\"$JOB\"}" \
  --endpoint-url "$EP"

aws batch submit-job --job-name job1 --job-queue lab-jq --job-definition lab-jd --endpoint-url "$EP"
```

## Not yet / deferred

- Array jobs, multi-node parallel jobs, fair-share scheduling
- Fargate or EC2 capacity provider fidelity beyond nested DinD
- Retry strategies, timeouts, and scheduling policy depth
- Rootless / deprivileged nested engine for jobs
