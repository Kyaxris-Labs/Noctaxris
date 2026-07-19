# Noctaxris docs

Public reference for **Noctaxris** (module `github.com/Kyaxris-Labs/Noctaxris`). Product name is PascalCase `Noctaxris`.

Noctaxris is a Docker-first AWS-shaped emulator for cloud security labs. The lab core is shipped: loopback by default, no host `docker.sock`, sealed secrets and CMK material, SigV4, lab IAM, full STS with fail-closed federation, Organizations MVP, lab-complete KMS/S3/DynamoDB/SQS/Lambda. Remaining depth and new lab cores are tracked on the per-service pages.

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
| [services/index.md](services/index.md#cross-cutting) | Cross-cutting deferred (condition keys, post-v2 microVMs, planned cores) |

Quick start stays in the root [README](../README.md).
