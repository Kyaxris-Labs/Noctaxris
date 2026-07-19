# Noctaxris docs

Public reference for **Noctaxris** (repo and Go module: `github.com/Kyaxris-Labs/Noctaxris`). The name is PascalCase `Noctaxris`, not all-lowercase `noctaxris`.

Noctaxris is a local AWS-shaped emulator aimed at cloud security labs. It ships as a Docker image first. The process listens on loopback by default, refuses host `docker.sock`, encrypts access-key secrets at rest, verifies SigV4, and writes CloudTrail-shaped audit lines.

| Doc | What it covers |
|-----|----------------|
| [architecture.md](architecture.md) | Packages, startup, request path |
| [configuration.md](configuration.md) | Env vars, Compose, data files |
| [security-defaults.md](security-defaults.md) | Hard defaults and auth posture |
| [phase-0.md](phase-0.md) | Scaffold history |
| [phase-1.md](phase-1.md) | SigV4 + GetCallerIdentity |

Quick start stays in the root [README](../README.md).
