# CodeBuild

**Status:** shipped

Lab CodeBuild core: project create, StartBuild on nested DinD, BatchGetBuilds, and ListBuilds. JSON protocol via `X-Amz-Target: CodeBuild_20161006.*`. Builds reuse the nested Docker client (same TLS DinD host as Lambda and ECS). No host `docker.sock`.

## Implemented

| Area | Actions |
|------|---------|
| Projects | `CreateProject` (source `NO_SOURCE` or `S3`, inline buildspec, environment image, `NO_ARTIFACTS`) |
| Builds | `StartBuild`, `BatchGetBuilds`, `ListBuilds` |
| Roles | `CreateProject` requires `serviceRole`. Caller needs `iam:PassRole`. Role trust must Allow `sts:AssumeRole` for `codebuild.amazonaws.com`. StartBuild mints temporary AWS_* credentials for the project `serviceRole` into the nested container |
| Compute | Nested containers via Compose `noctaxris-engine` (DinD TLS). StartBuild reuses the nested ECS run helper on Internal network `noctaxris-ecs` (host-gateway ExtraHosts off by default). Lab registry image refs (`127.0.0.1:4566/...`) are rewritten and pulled with a registry token before start |

### Authz notes

CodeBuild APIs use identity `EvaluateFull` on project ARNs where applicable.

`CreateProject` calls `CheckPassRole` with service principal `codebuild.amazonaws.com`.

Without `NOCTAXRIS_DOCKER_HOST`, `StartBuild` returns compute unavailable.

### Source and buildspec

- `NO_SOURCE` requires inline `source.buildspec` (JSON buildspec preferred, YAML-ish `commands:` lists accepted).
- `S3` requires `source.location` as `bucket/key` or `s3://bucket/key`. When buildspec is empty, the object body is used as the buildspec.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine`. Skip live StartBuild when Docker is unavailable (unit tests cover PassRole and compute-unavailable).

```bash
CB_TRUST='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"codebuild.amazonaws.com"},"Action":"sts:AssumeRole"}]}'

aws iam create-role --role-name LabCodeBuildRole --assume-role-policy-document "$CB_TRUST" --endpoint-url "$EP"
ROLE=$(aws iam get-role --role-name LabCodeBuildRole --endpoint-url "$EP" --query Role.Arn --output text)

aws codebuild create-project \
  --name noctaxris-lab \
  --service-role "$ROLE" \
  --source type=NO_SOURCE,buildspec='{"version":"0.2","phases":{"build":{"commands":["echo codebuild-ok"]}}}' \
  --artifacts type=NO_ARTIFACTS \
  --environment type=LINUX_CONTAINER,image=alpine:3.20,computeType=BUILD_GENERAL1_SMALL \
  --endpoint-url "$EP"

aws codebuild start-build --project-name noctaxris-lab --endpoint-url "$EP"
aws codebuild list-builds --endpoint-url "$EP"
```

## Not yet / deferred

- VPC config, fleets, privileged mode depth, cache, secondary sources
- Batch build matrix and CodeCommit
- CloudWatch Logs and S3 artifact publishing beyond lab status records
- Rootless / deprivileged nested engine for builds
