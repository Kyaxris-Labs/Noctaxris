# ECS

**Status:** shipped

Lab-complete ECS core: task definitions (with required task and execution roles), RunTask on nested DinD, task list/describe/stop, and default cluster metadata. JSON protocol via `X-Amz-Target: AmazonECS*` or service `ecs` with JSON body. Platform is nested Docker inside `noctaxris-engine`, not Fargate or EC2 capacity providers.

## Implemented

| Area | Actions |
|------|---------|
| Task definitions | `RegisterTaskDefinition`, `DescribeTaskDefinition`, `ListTaskDefinitions`, `DeregisterTaskDefinition` |
| Tasks | `RunTask`, `DescribeTasks`, `ListTasks`, `StopTask` |
| Services | `CreateService`, `UpdateService`, `DeleteService`, `DescribeServices`, `ListServices` with DesiredCount lab reconciler (start/stop nested tasks toward desired). No awsvpc ENI |
| Clusters | `DescribeClusters`, `ListClusters` (default cluster `default` seeded per account) |
| Roles | `RegisterTaskDefinition` and `RunTask` require `taskRoleArn` **and** `executionRoleArn`. Caller needs `iam:PassRole` on each role. Role trust must Allow `sts:AssumeRole` for `ecs-tasks.amazonaws.com` |
| Compute | Nested containers via Compose `noctaxris-engine` (DinD, TLS on port 2376). Tasks run on Internal network `noctaxris-ecs` without host-gateway ExtraHosts by default (`NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1` to opt in). Nested tasks drop all Linux capabilities (`CapDrop: ALL`). No host `docker.sock` on the API container. After the container exits, task status becomes `STOPPED` (background reaper plus sync on `DescribeTasks` / `ListTasks`). Live RunTask requires a healthy engine |
| Task role session | Temporary AWS_* credentials for the task role injected into the container (same mint pattern as Lambda Invoke) |
| Lab registry images | Task definitions may reference `127.0.0.1:4566/ACCOUNT/REPO:tag`. RunTask pulls inside DinD using a registry token when the image uses the lab ECR host |

Task definitions, tasks, and cluster metadata live in SQLite.

### Authz notes

ECS control-plane APIs use identity `EvaluateFull` on cluster, task-definition, or task ARNs.

`RegisterTaskDefinition` and `RunTask` call `CheckPassRole` for both role ARNs with service principal `ecs-tasks.amazonaws.com`.

Without `NOCTAXRIS_DOCKER_HOST`, `RunTask` returns compute unavailable.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine`. Push a lab image first (see [ecr.md](ecr.md#how-to-verify--cli-smoke)).

```bash
ACCOUNT=$(aws sts get-caller-identity --endpoint-url "$EP" --query Account --output text)
REPO="noctaxris-lab-$RANDOM"

aws ecr create-repository --repository-name "$REPO" --endpoint-url "$EP"
PASS=$(aws ecr get-login-password --endpoint-url "$EP")
echo "$PASS" | docker login --username AWS --password-stdin 127.0.0.1:4566
docker pull alpine:3.20
docker tag alpine:3.20 "127.0.0.1:4566/${ACCOUNT}/${REPO}:lab"
docker push "127.0.0.1:4566/${ACCOUNT}/${REPO}:lab"

ECS_TRUST='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}'

aws iam create-role --role-name LabECSTaskRole --assume-role-policy-document "$ECS_TRUST" --endpoint-url "$EP"
aws iam create-role --role-name LabECSExecRole --assume-role-policy-document "$ECS_TRUST" --endpoint-url "$EP"

TASK_ROLE=$(aws iam get-role --role-name LabECSTaskRole --endpoint-url "$EP" --query Role.Arn --output text)
EXEC_ROLE=$(aws iam get-role --role-name LabECSExecRole --endpoint-url "$EP" --query Role.Arn --output text)

IMAGE="127.0.0.1:4566/${ACCOUNT}/${REPO}:lab"
CONTAINERS="[{\"name\":\"app\",\"image\":\"${IMAGE}\",\"essential\":true,\"command\":[\"echo\",\"ecs-ok\"]}]"

aws ecs register-task-definition \
  --family noctaxris-lab \
  --task-role-arn "$TASK_ROLE" \
  --execution-role-arn "$EXEC_ROLE" \
  --container-definitions "$CONTAINERS" \
  --endpoint-url "$EP"

aws ecs run-task --cluster default --task-definition noctaxris-lab --endpoint-url "$EP"
aws ecs list-tasks --cluster default --endpoint-url "$EP"
```

CreateService with DesiredCount 0 (no nested start). Scale DesiredCount up only when DinD is available.

```bash
aws ecs create-service \
  --cluster default \
  --service-name noctaxris-svc \
  --task-definition noctaxris-lab \
  --desired-count 0 \
  --endpoint-url "$EP"
aws ecs list-services --cluster default --endpoint-url "$EP"
```

Expect `register-task-definition` to fail without both role ARNs. Expect `run-task` to return a task ARN when DinD is up. Expect `list-tasks` to include the task. After a short-lived command exits, `describe-tasks` should show `STOPPED` without calling `stop-task`.

## Not yet / deferred

- Load balancers, `awsvpc` networking, capacity providers, ECS Exec, Service Connect
- Autoscaling, circuit breakers, placement constraints, EBS volumes, Firelens matrix
- Cross-account or multi-cluster depth beyond same-account `default`
- Rootless / deprivileged nested engine (privilege reduction planned; see [security-defaults.md](../security-defaults.md))
