# Noctaxris docs

Public reference for **Noctaxris** (module `github.com/Kyaxris-Labs/Noctaxris`). Product name is PascalCase `Noctaxris`.

Noctaxris is a Docker-first AWS-shaped emulator for cloud security labs. Lab cores are shipped: loopback by default, no host `docker.sock`, sealed secrets and CMK material, SigV4 (with documented auth-open exceptions including Cognito `InitiateAuth`), multi-account dual eval and OU SCP/RCP inheritance, identity (IAM/STS/Organizations), crypto and data plane with versioning/GSI/rotation depth and S3 bucket notifications, SSM and Secrets Manager, Lambda with FilterCriteria ESM and lab ECR Image pull on nested DinD, SNS, EventBridge (content filters; RoleArn or resource-policy delivery for SQS/Lambda/SNS; Scheduler and Pipes), ECR, ECS, CloudTrail, CloudWatch Logs, Resource Groups Tagging API, Kinesis, Firehose, SES, AppConfig, Step Functions, CloudFormation (policy/permission/notification types), CodeBuild, CodePipeline, Batch, Glue, WAF v2, Config, Cognito User Pools, API Gateway HTTP API (`GetIntegrations`/`GetRoutes`/`GetAuthorizers`; `/v2/apis` before Registry `/v2/`), AppSync, edge/governance stubs, nested RDS/ElastiCache/DocumentDB when DinD is up, RDS Data API (`pgx` against the nested data-plane DSN with nested-`psql` fallback, otherwise `DatabaseUnavailableException`), Athena (Glue + lab S3 SELECT subset), nested OpenSearch when DinD is up (else fail-closed `CreateFailed` + `stub://` + lab `FailureReason` on mmap/lock), EMR control-plane stub, and Bedrock/Textract/Transcribe canned stubs. Default Compose runs restricted DinD (`privileged: false` + explicit caps). Remaining deferred depth is tracked on the per-service pages.

## Reference

| Doc | Topic |
|-----|--------|
| [services/index.md](services/index.md) | Per-service APIs, authz, CLI smoke, deferred depth |
| [architecture.md](architecture.md) | Deploy graph, packages, and request path |
| [configuration.md](configuration.md) | Env vars, data layout, Compose, backup, IdP bootstrap |
| [ops.md](ops.md) | Single-replica rule, backup/restore, upgrades, CI matrix, Hub images |
| [release.md](release.md) | Cut a semver release and Docker Hub tags |
| [security-defaults.md](security-defaults.md) | Host, crypto, and auth posture |
| [../tests/README.md](../tests/README.md) | SDK (Go/Node/Python) / Terraform / CloudFormation integration suites |
| [../CHANGELOG.md](../CHANGELOG.md) | Lab-core release notes |

## History

| Doc | Topic |
|-----|--------|
| [history/index.md](history/index.md) | Lab-core delivery history (not a live roadmap) |
| [services/index.md](services/index.md#cross-cutting) | Cross-cutting notes (dual eval, OU inheritance, condition keys, compute runtime) |

Quick start stays in the root [README](../README.md).
