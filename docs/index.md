# Noctaxris docs

Public reference for **Noctaxris** (module `github.com/Kyaxris-Labs/Noctaxris`). Product name is PascalCase `Noctaxris`.

Noctaxris is a Docker-first AWS-shaped emulator for cloud security labs. The lab core (phases 0-7) is shipped: loopback by default, no host `docker.sock`, sealed secrets and CMK material, SigV4, lab IAM, full STS with fail-closed federation, Organizations MVP, lab-complete KMS/S3/DynamoDB/SQS/Lambda.

## Reference

| Doc | Topic |
|-----|--------|
| [architecture.md](architecture.md) | Packages and request path |
| [configuration.md](configuration.md) | Env vars, data layout, Compose, IdP bootstrap |
| [security-defaults.md](security-defaults.md) | Host, crypto, and auth posture |
| [verification.md](verification.md) | Local tests and Compose smoke |
| [../CHANGELOG.md](../CHANGELOG.md) | Lab-core ship notes |

## Roadmap

| Doc | Topic |
|-----|--------|
| [phases/index.md](phases/index.md) | Phase 0-7 history (lab core complete) |
| [deferred.md](deferred.md) | IAM/STS/KMS/S3/DynamoDB/SQS/Lambda depth left for later |

Quick start stays in the root [README](../README.md).
