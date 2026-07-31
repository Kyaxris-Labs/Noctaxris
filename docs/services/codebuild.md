# CodeBuild

**Status:** shipped

Lab CodeBuild core: project create/list/get/update/delete, StartBuild / StartBuildBatch on nested DinD (with env overrides and override-lock), BatchGetBuilds, ListBuilds, StopBuild, and S3 artifact upload on successful reap. JSON protocol via `X-Amz-Target: CodeBuild_20161006.*`. Builds reuse the nested Docker client (same TLS DinD host and `noctaxris-ecs` path as ECS). No host `docker.sock`.

## Implemented

| Area | Actions |
|------|---------|
| Projects | `CreateProject`, `ListProjects`, `BatchGetProjects`, `UpdateProject`, `DeleteProject` (source `NO_SOURCE`, `S3`, or `CODECOMMIT`; inline buildspec; environment image; project `environment.environmentVariables`; `source.allowOverride`; artifacts `NO_ARTIFACTS` or `S3`) |
| Builds | `StartBuild` (including `environmentVariablesOverride` / `buildspecOverride`), `StartBuildBatch` (matrix or buildspec `batch.build-list` / `batch.build-matrix`), `BatchGetBuilds`, `ListBuilds`, `StopBuild` |
| Override-lock | When `source.allowOverride` is `false`, `StartBuild` / `StartBuildBatch` reject non-empty `buildspecOverride` or `environmentVariablesOverride` with `InvalidInputException` (matrix children from project buildspec still allowed) |
| Artifacts | When project `artifacts.type=S3` (bucket in `location`, optional `path`/`name`/`packaging`), successful reap uploads a ZIP (or text/tar) to S3 and sets build `artifacts.location`. Prefer `CopyFromContainer` of `/codebuild/src` (validated under the CodeBuild workspace root) before container stop; fall back to Exec+tar+base64 when copy fails or the workspace is empty. Logs-derived `build.log` when no workspace archive |
| CODECOMMIT source | `source.type=CODECOMMIT` with `source.location` as a lab repository name or CodeCommit ARN. StartBuild materializes the lab tree (`MaterializeCodeCommitRepo`) and injects files with `CopyToContainer` into `/codebuild/src` after container create and before start (no base64 in the start command). Missing repos fail closed with `InvalidInputException` before compute. Empty project buildspec loads `buildspec.yml` / `.yaml` / `.json` from the repo when present |
| Config stubs | `vpcConfig`, `cache`, `secondarySources`, `fleet` / `projectFleet`, and `reportGroupArns` persist as JSON and echo on Create/Update/BatchGetProjects (decorative `status=ACTIVE` on vpc/cache/fleet). Validated shapes: `cache.type` in `NO_CACHE`/`LOCAL`/`S3`; `vpcConfig.vpcId` + non-empty `subnets`; `fleet.fleetArn` ARN; `reportGroupArns` ARN list. Config-only: no real VPC attach, cache backend, fleets, or report publishing |
| Lab image | Optional tool-bearing image under `docker/codebuild-lab/` (see [codebuild-lab-image.md](codebuild-lab-image.md)); pull via lab registry refs already allowed by the DinD allowlist |
| IMDS mirror | Nested builds started via `RunECSTask` with minted `AWS_*` also get link-local `169.254.170.2` container-credential env on Internal `noctaxris-ecs` (not published on the host API listen). Prefer `AWS_CONTAINER_CREDENTIALS_FULL_URI` (`http://169.254.170.2:9254/v2/credentials/...`); relative URI alone assumes port 80 and will not hit the lab mirror. `AWS_CONTAINER_CREDENTIALS_RELATIVE_URI` is set alongside FULL_URI |
| Webhooks | AWS-shaped `CreateWebhook` / `DeleteWebhook` / `ListWebhooks` (JSON protocol) plus receive path `POST /_noctaxris/codebuild/webhook/{account}/{project}` with lite `filterGroups` (`EVENT` / `HEAD_REF` / `FILE_PATH`) into StartBuild. `payloadUrl` points at the lab receive path (not GitHub SaaS). Optional secret via request or auto-minted; receive checks `X-Noctaxris-Webhook-Secret` when set |
| Roles | `CreateProject` / `UpdateProject` require `serviceRole`. Caller needs `iam:PassRole`. Role trust must Allow `sts:AssumeRole` for `codebuild.amazonaws.com`. StartBuild mints temporary AWS_* credentials for the project `serviceRole` into the nested container. Lab registry pull tokens use the service role ARN |
| Compute | Nested containers via Compose `noctaxris-engine` (DinD TLS). StartBuild starts the container and returns `IN_PROGRESS`; exit is reaped in the background. Lab registry image refs (`127.0.0.1:4566/...`) are rewritten and pulled once with authenticated Registry V2 |
| Endpoint mint | Nested build env includes `AWS_ENDPOINT_URL_*` aliases for Secrets Manager (`AWS_ENDPOINT_URL_SECRETSMANAGER`, `AWS_ENDPOINT_URL_SECRETS_MANAGER`), CloudWatch Logs (`AWS_ENDPOINT_URL_LOGS`), SNS (`AWS_ENDPOINT_URL_SNS`), and CodeBuild (`AWS_ENDPOINT_URL_CODEBUILD`), plus the shared mint set used by ECS-path compute |
| Logs | On reap, container stdout/stderr land on the build row and CloudWatch Logs lite under group `/aws/codebuild/<project>`; `BatchGetBuilds` surfaces log group/stream |

### Authz notes

CodeBuild APIs use identity `EvaluateFull` on project ARNs where applicable.

`CreateProject` and `UpdateProject` call `CheckPassRole` with service principal `codebuild.amazonaws.com`.

Without `NOCTAXRIS_DOCKER_HOST`, `StartBuild` / `StartBuildBatch` return compute unavailable.

### Source and buildspec

- `NO_SOURCE` requires inline `source.buildspec` (JSON buildspec preferred, YAML-ish `commands:` lists accepted).
- `S3` requires `source.location` as `bucket/key` or `s3://bucket/key`. When buildspec is empty, the object body is used as the buildspec.
- `CODECOMMIT` requires `source.location` as a repository name or `arn:aws:codecommit:REGION:ACCOUNT:NAME`. Inline buildspec optional when the repo contains `buildspec.yml` (or `.yaml` / `.json`).
- `source.allowOverride` defaults to `true`. Set `false` to lock out StartBuild overrides.

### StartBuildBatch (lab-lite)

- Request `matrix` (array of env-var arrays or objects) or `buildList`, or parse project/override buildspec `batch.build-list` / `batch.build-matrix` (static rows or dynamic env cartesian product, capped at 32).
- Creates a parent batch row plus N child builds that share the project service role; response includes `buildBatch` with `buildGroups`.

### Host reachability

CodeBuild nested containers use the same DinD path as ECS (`noctaxris-ecs`). Host-gateway ExtraHosts stay off by default. Set `NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1` or use `docker/compose.lab-ecs-host-gateway.yaml` so builds can reach the host-published API (see [ops.md](../ops.md#compose-overlays-lab-opt-in)). On Docker Desktop DinD, pair that session with `NOCTAXRIS_PUBLISH_ADDR=0.0.0.0` if loopback publish is unreachable via host-gateway.

### Lab webhooks

Register via `CreateWebhook` (`projectName`, optional `filterGroups`, optional `secret`). Response includes `webhook.payloadUrl` / `webhook.url` pointing at `POST /_noctaxris/codebuild/webhook/{account}/{project}` and a `secret` (auto-minted when omitted). `ListWebhooks` (optional `projectName` filter) and `DeleteWebhook` manage rows. Store upsert remains available for operators/tests.

Receive (unauthenticated lab path; optional shared secret):

```http
POST /_noctaxris/codebuild/webhook/{account}/{project}
Content-Type: application/json
X-Noctaxris-Webhook-Secret: <optional; required when the webhook secret is non-empty>

{"event":"PUSH","headRef":"refs/heads/main","filePaths":["src/app.go"]}
```

Body fields: `event` (required), `headRef` or `head_ref`, `filePaths` or `file_paths`. Filter groups support `EVENT`, `HEAD_REF`, and `FILE_PATH` (lite). Matched posts call StartBuild; unmatched filters return `{"triggered":false}`. Without `NOCTAXRIS_DOCKER_HOST`, a matched post returns 503 and does not create a build row. Missing webhook rows return HTTP 404.

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

aws codebuild list-projects --endpoint-url "$EP"
aws codebuild batch-get-projects --names noctaxris-lab --endpoint-url "$EP"
aws codebuild start-build --project-name noctaxris-lab --endpoint-url "$EP"
aws codebuild list-builds --endpoint-url "$EP"
```

## Not yet / deferred

- Full GitHub SaaS / CodeBreach incident clone claims (lab webhooks stay local HTTP only)
- Real VPC networking, cache backends, fleets, or report-group execution (validated config stubs only)
