# Noctaxris docs

Public reference for **Noctaxris** (module `github.com/Kyaxris-Labs/Noctaxris`). Product name is PascalCase `Noctaxris`.

Noctaxris is a Docker-first AWS-shaped emulator for cloud security labs. It listens on loopback by default, refuses host `docker.sock`, seals secrets and CMK material at rest, verifies SigV4, and ships lab IAM, full STS routing with fail-closed federation, lab-complete KMS, lab-complete S3, and Organizations MVP.

## Reference

| Doc | Topic |
|-----|--------|
| [architecture.md](architecture.md) | Packages and request path |
| [configuration.md](configuration.md) | Env vars, data layout, Compose, IdP bootstrap |
| [security-defaults.md](security-defaults.md) | Host, crypto, and auth posture |
| [verification.md](verification.md) | Local tests and Compose smoke |

## Roadmap

| Doc | Topic |
|-----|--------|
| [phases/index.md](phases/index.md) | Phase 0–5 history |
| [deferred.md](deferred.md) | IAM/STS/KMS/S3 depth left for later |

Quick start stays in the root [README](../README.md).
