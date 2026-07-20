# Noctaxris docs

Public reference for **Noctaxris** (module `github.com/Kyaxris-Labs/Noctaxris`). Product name is PascalCase `Noctaxris`.

Noctaxris is a Docker-first AWS-shaped emulator for cloud security labs. Lab cores through v4 are shipped: loopback by default, no host `docker.sock`, sealed secrets and CMK material, SigV4, multi-account dual eval and OU SCP/RCP inheritance, identity (IAM/STS/Organizations), crypto and data plane with versioning/GSI/rotation depth, SSM and Secrets Manager, Lambda with lab ECR Image pull and opt-in microVM selection (DinD default), SNS, EventBridge, ECR, ECS, CloudTrail, CloudWatch Logs, Resource Groups Tagging API, Kinesis, Firehose, SES, AppConfig, Step Functions, CloudFormation, CodeBuild, CodePipeline, Batch, Glue, WAF v2, and Config. Remaining deferred depth (including live Firecracker guest boot and Athena) is tracked on the per-service pages.

## Reference

| Doc | Topic |
|-----|--------|
| [services/index.md](services/index.md) | Per-service APIs, authz, CLI smoke, deferred depth |
| [architecture.md](architecture.md) | Deploy graph, packages, and request path |
| [configuration.md](configuration.md) | Env vars, data layout, Compose, IdP bootstrap |
| [security-defaults.md](security-defaults.md) | Host, crypto, and auth posture |
| [../CHANGELOG.md](../CHANGELOG.md) | Lab-core through v4 notes |

## Roadmap

| Doc | Topic |
|-----|--------|
| [phases/index.md](phases/index.md) | Lab-core delivery history |
| [services/index.md](services/index.md#cross-cutting) | Cross-cutting notes (dual eval, OU inheritance, condition keys, compute runtime) |

Quick start stays in the root [README](../README.md).
