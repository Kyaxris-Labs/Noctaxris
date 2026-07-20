# Scaffold (historical)

Historical ship snapshot: the runnable scaffold. It proves packaging, persistence, audit plumbing, and a fail-closed HTTP gate before real AWS API fidelity.

## Delivered

| Area | Status |
|------|--------|
| Go module `github.com/Kyaxris-Labs/Noctaxris` | Done |
| Config from env with localhost default listen | Done |
| Master key + Seal/Unseal | Done |
| SQLite identity store + encrypted root | Done |
| CloudTrail-shaped JSONL audit | Done |
| Health + auth gate + NotImplemented stub | Done |
| Optional TLS when cert and key set | Done |
| Docker multi-stage + Compose secure defaults | Done |
| Security regression tests (compose sock, unauth 403 + audit) | Done |

## Explicitly deferred at this ship

- Full SigV4 signature verification (header and query)
- IAM policy evaluation
- Any STS/IAM/S3/KMS/DynamoDB/SQS/Lambda API beyond the stub response
- Multi-account / Organizations
- Nested containers or microVMs for compute

## How to verify

Shared Compose and `go test`: [../services/index.md](../services/index.md#shared-verification).

## Naming note

| Kind | Spelling |
|------|----------|
| Repo / module / product name | `Noctaxris` |
| Go import path | `github.com/Kyaxris-Labs/Noctaxris` |
| Binary package dir | `cmd/noctaxris` |
| Env prefix / data path / health URL | lowercase `noctaxris` / `NOCTAXRIS_*` |

Keep the product and module as `Noctaxris`. Lowercase forms are for Unix paths, env names, and the CLI binary folder only.
