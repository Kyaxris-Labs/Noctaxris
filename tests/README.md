# Integration tests

Real SDK, Terraform, and CloudFormation suites against a running Noctaxris API. These are not docs-only stubs.

Endpoint default: `http://127.0.0.1:4566`. Use the same root keys as `docker/.env`.

## Prerequisites

1. Compose up and ready:

```bash
cp docker/.env.example docker/.env
docker compose -f docker/compose.yaml --env-file docker/.env up --build -d

curl -fsS http://127.0.0.1:4566/_noctaxris/health
curl -fsS http://127.0.0.1:4566/_noctaxris/ready
```

2. Export credentials (match `docker/.env`):

```bash
export AWS_ACCESS_KEY_ID=AKIAROOTEXAMPLE01
export AWS_SECRET_ACCESS_KEY='wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'
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
- Terraform needs the Terraform binary on `PATH`. The runner skips when it is missing.
- Prefer WSL or Linux for AWS CLI and Terraform against `127.0.0.1:4566` when Docker Desktop publishes that port on the Windows host.

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
