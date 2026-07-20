# Noctaxris docs

Public reference for **Noctaxris** (module `github.com/Kyaxris-Labs/Noctaxris`). Product name is PascalCase `Noctaxris`.

Noctaxris is a Docker-first AWS-shaped emulator for cloud security labs. The v1 lab core and v2 lab cores are shipped: loopback by default, no host `docker.sock`, sealed secrets and CMK material, SigV4, identity (IAM/STS/Organizations), crypto and data plane, SSM and Secrets Manager, Lambda, SNS, EventBridge, ECR, and ECS. Remaining deferred depth and post-v2 work are tracked on the per-service pages.

## Reference

| Doc | Topic |
|-----|--------|
| [services/index.md](services/index.md) | Per-service APIs, authz, CLI smoke, deferred depth |
| [architecture.md](architecture.md) | Packages and request path |
| [configuration.md](configuration.md) | Env vars, data layout, Compose, IdP bootstrap |
| [security-defaults.md](security-defaults.md) | Host, crypto, and auth posture |
| [../CHANGELOG.md](../CHANGELOG.md) | Lab-core and v2 notes |

## Roadmap

| Doc | Topic |
|-----|--------|
| [phases/index.md](phases/index.md) | Lab-core delivery history |
| [services/index.md](services/index.md#cross-cutting) | Cross-cutting deferred (condition-key operator depth, post-v2 microVMs) |

Quick start stays in the root [README](../README.md).
