# Noctaxris docs

Public reference for **Noctaxris** (module `github.com/Kyaxris-Labs/Noctaxris`). Product name is PascalCase `Noctaxris`.

Noctaxris is a Docker-first AWS-shaped emulator for cloud security labs. The v1 lab core, v2 lab cores, and v3 expansion are shipped: loopback by default, no host `docker.sock`, sealed secrets and CMK material, SigV4, multi-account dual eval and OU SCP/RCP inheritance, identity (IAM/STS/Organizations), crypto and data plane, SSM and Secrets Manager, Lambda with lab ECR Image pull, SNS, EventBridge, ECR, ECS, CloudTrail, CloudWatch Logs, Resource Groups Tagging API, Kinesis, SES, AppConfig, and Step Functions. Remaining deferred depth and later microVM work are tracked on the per-service pages.

## Reference

| Doc | Topic |
|-----|--------|
| [services/index.md](services/index.md) | Per-service APIs, authz, CLI smoke, deferred depth |
| [architecture.md](architecture.md) | Packages and request path |
| [configuration.md](configuration.md) | Env vars, data layout, Compose, IdP bootstrap |
| [security-defaults.md](security-defaults.md) | Host, crypto, and auth posture |
| [../CHANGELOG.md](../CHANGELOG.md) | Lab-core, v2, and v3 notes |

## Roadmap

| Doc | Topic |
|-----|--------|
| [phases/index.md](phases/index.md) | Lab-core delivery history |
| [services/index.md](services/index.md#cross-cutting) | Cross-cutting notes (dual eval, OU inheritance, condition keys, post-v2 microVMs) |

Quick start stays in the root [README](../README.md).
