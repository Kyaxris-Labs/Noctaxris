# Integration tests

Real SDK, Terraform, and CloudFormation suites against a running Noctaxris API. These are not docs-only stubs.

Endpoint default: `http://127.0.0.1:4566`. Use the same root keys as `docker/.env`.

## Prerequisites

1. Compose up and ready (Compose binds `0.0.0.0` in-container; the shipped `.env.example` root pair is refused):

```bash
cp docker/.env.example docker/.env
ROOT_AKID="AKIANOCTAXRIS$(openssl rand -hex 6 | tr '[:lower:]' '[:upper:]')"
ROOT_SECRET="$(openssl rand -hex 32)"
awk -v akid="$ROOT_AKID" -v secret="$ROOT_SECRET" '
  /^NOCTAXRIS_ROOT_ACCESS_KEY_ID=/ { print "NOCTAXRIS_ROOT_ACCESS_KEY_ID=" akid; next }
  /^NOCTAXRIS_ROOT_SECRET_ACCESS_KEY=/ { print "NOCTAXRIS_ROOT_SECRET_ACCESS_KEY=" secret; next }
  { print }
' docker/.env > docker/.env.tmp && mv docker/.env.tmp docker/.env

docker compose -f docker/compose.yaml --env-file docker/.env up --build -d

curl -fsS http://127.0.0.1:4566/_noctaxris/health
curl -fsS http://127.0.0.1:4566/_noctaxris/ready
```

Or copy `.env.example` and replace both `NOCTAXRIS_ROOT_*` values yourself before `compose up`.

2. Export credentials (match `docker/.env`):

```bash
set -a && source docker/.env && set +a
export AWS_ACCESS_KEY_ID="$NOCTAXRIS_ROOT_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$NOCTAXRIS_ROOT_SECRET_ACCESS_KEY"
export AWS_DEFAULT_REGION=us-east-1
export AWS_EC2_METADATA_DISABLED=true
export NOCTAXRIS_ENDPOINT=http://127.0.0.1:4566
```

Optional overrides: `NOCTAXRIS_ENDPOINT`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_DEFAULT_REGION`.

| Suite | Tools |
|-------|--------|
| SDK (Go) | Go 1.22+ (module under `tests/sdk/go`) |
| SDK (Node.js) | Node.js 24+; `npm install` under `tests/sdk/nodejs` |
| SDK (Python) | Python 3.10+; `pip install -r requirements.txt` under `tests/sdk/python` |
| Terraform | Terraform CLI 1.5+, AWS provider resolved on `init` |
| CloudFormation | Go SDK suite and/or AWS CLI v2 |

## Run all suites

From the repo root (bash / WSL / Git Bash):

```bash
bash tests/run-all.sh

# Advanced fullstack + Terraform lab-fullstack (longer)
NOCTAXRIS_ADVANCED=1 bash tests/run-all.sh

# Terraform lab-fullstack only (after lab-core), without full ADVANCED SDK
TF_FULLSTACK=1 bash tests/run-all.sh

# Nested Lambda Invoke in SDK suites (Compose noctaxris-engine healthy)
NOCTAXRIS_NESTED=1 bash tests/run-all.sh
```

Or run each suite:

```bash
# SDK round-trips (STS, S3, DynamoDB, IAM, KMS, SQS, SNS, Lambda CRUD, EventBridge, SSM, Secrets)
cd tests/sdk/go && go test ./... -count=1 -timeout 10m

cd tests/sdk/nodejs && npm install && npm test

cd tests/sdk/python && pip install -r requirements.txt && pytest

# Terraform apply + destroy (lab-core: S3 + IAM + DynamoDB + KMS)
bash tests/terraform/run.sh

# Terraform advanced stack (IAM, KMS, S3, DynamoDB, SQS, SNS, Lambda zip, EventBridge, SSM, Secrets)
STACK=lab-fullstack bash tests/terraform/run.sh

# CloudFormation CreateStack / Describe / Delete (JSON or YAML lab subset)
cd tests/cloudformation/go && go test ./... -count=1 -timeout 5m
# or: bash tests/cloudformation/run.sh
```

### Unreachable API behavior

| Suite | Default when API down | Optional skip |
|-------|----------------------|---------------|
| Go SDK | `t.Skip` | (always skips) |
| Node / Python SDK | fail with a clear message | set `NOCTAXRIS_SKIP_IF_DOWN=1` |
| Terraform / `run-all.sh` | exit non-zero with message | n/a |

Failures on assertions happen only when the endpoint is up.

## What is covered

| Suite | Coverage |
|-------|----------|
| `tests/sdk/go` | Create/list/get/delete round-trips with unique prefixes and cleanup |
| `tests/sdk/nodejs` | Same service set via AWS SDK for JavaScript v3 (`node:test`) |
| `tests/sdk/python` | Same service set via boto3 + pytest |
| `tests/terraform` | Real HCL, custom `endpoints` to Noctaxris; `lab-core` (S3 + IAM + DynamoDB + KMS) and `STACK=lab-fullstack` multi-resource apply then destroy |
| `tests/cloudformation` | JSON/YAML lab templates for S3, IAM Role, SQS, DynamoDB, Lambda |

## Honest limitations

- CloudFormation lab subset: JSON or YAML; ChangeSet lite; nested stacks (lab S3 TemplateURL); drift lite; resources toward lab-fullstack (S3, IAM Role, SQS(+QueuePolicy), DynamoDB, Lambda ZipFile, KMS, SNS, Events, SSM, Secrets, nested Stack); DependsOn + `Ref`/`Fn::GetAtt`/`Fn::Sub`/`Fn::Join`. Unknown types/props fail closed.
- Default Lambda SDK tests cover Create/Get/List/Delete. Live Invoke is opt-in (`NOCTAXRIS_NESTED=1`) and needs nested DinD.
- Edge / data / workflow / nested SDK rows (CloudFront edge, Transfer files, Glue crawler, AppConfig deploy, Config history, SFN Choice, CloudTrail delivery, Route53 Alias, ELBv2 rules, AppSync PassRole) run whenever the API is up. Nested OpenSearch / ActiveMQ / Firehose OpenSearch / Lambda MQ ESM rows skip when engines are not Active/RUNNING.
- Forensic depth surfaces (CloudTrail inject/Insights, GuardDuty/Security Hub/Macie/Detective, VPC Flow inject, Object Lock / access logs, lab clock/BulkSeed, and related) are covered by `go test ./internal/...` unit tests; they are not yet required rows in SDK `run-all.sh`.
- Terraform needs the Terraform binary on `PATH`. The runner skips when it is missing.
- Compose publishes `127.0.0.1:4566` only. When the API runs on a Windows host, WSL cannot reach that loopback; `tests/terraform/run.sh` skips automatically when the endpoint host is `127.0.0.1` or `localhost` (override with `NOCTAXRIS_FORCE_WSL_TF=1` only if EP is reachable). Do not widen Compose publish. Run AWS CLI / Terraform from a host that shares the loopback with Compose (Windows host for Docker Desktop, or Linux where Compose listens locally).

Gaps and follow-ups: [HANDOFF.md](HANDOFF.md).

## Advanced suites

Multi-service **lab order pipeline** (IAM, KMS, S3, DynamoDB, SQS, SNS, Lambda CRUD, EventBridge → queue/topic, SSM, Secrets Manager). SDK suites are gated so default `npm test` / `pytest` / `go test` stay fast. Terraform fullstack is opt-in via `STACK=lab-fullstack`. Both fold into `run-all.sh` when `NOCTAXRIS_ADVANCED=1`.

```bash
export NOCTAXRIS_ADVANCED=1
# same AWS_* / NOCTAXRIS_ENDPOINT as above

NOCTAXRIS_ADVANCED=1 bash tests/run-all.sh

# Terraform lab-fullstack only (after lab-core), without full ADVANCED SDK
TF_FULLSTACK=1 bash tests/run-all.sh

# Or individually:
cd tests/sdk/nodejs && npm install && node --test --test-name-pattern=fullstack test/fullstack.test.mjs
cd tests/sdk/python && pip install -r requirements.txt && pytest test_fullstack.py
cd tests/sdk/go && go test ./... -count=1 -timeout 10m -run Fullstack
STACK=lab-fullstack bash tests/terraform/run.sh
```

| Suite | Path | Status |
|-------|------|--------|
| Node.js SDK | `tests/sdk/nodejs/test/fullstack.test.mjs` | Implemented (gate `NOCTAXRIS_ADVANCED=1`) |
| Python SDK | `tests/sdk/python/test_fullstack.py` | Implemented (same gate) |
| Go SDK | `tests/sdk/go/fullstack_test.go` | Implemented (same gate) |
| Terraform | `tests/terraform/stacks/lab-fullstack/` | Implemented (`STACK=lab-fullstack` or `NOCTAXRIS_ADVANCED=1` in `run-all.sh`) |
| Nested Invoke | `*lambda*invoke*` per language | Implemented (gate `NOCTAXRIS_NESTED=1`) |

Assertions cover EventBridge → SQS delivery (including SourceArn queue policy), SNS → SQS fan-out, data-plane reads (S3/DDB/SSM SecureString/Secrets), empty delivery without an events queue policy, and SecureString decrypt denied when the CMK policy omits `kms:Decrypt`.

## CI

Optional GitHub Actions job `integration-suites` runs on `workflow_dispatch` or when paths under `tests/` change. Default path is the fast suite. `workflow_dispatch` can set `advanced_suites` / `nested_sdk`. Nested DinD smoke is weekly + manual, not every PR. See [docs/ops.md](../docs/ops.md).
