# Architecture (Phase 1)

One Go binary runs in the container. There is no sidecar API gateway and no Docker socket mount.

## Tree

```text
Noctaxris/
  cmd/noctaxris/               # process entrypoint
  docker/                      # Dockerfile, Compose, env example
  internal/config/             # LoadFromEnv
  internal/store/              # master key, Seal/Unseal, SQLite identity + policies
  internal/catalog/            # known actions for this phase
  internal/kernel/audit/       # CloudTrail-shaped JSONL writer
  internal/kernel/authn/       # SigV4 verify (header + query)
  internal/kernel/authz/       # single-account identity-policy Evaluate
  internal/kernel/identity/    # Principal model
  internal/kernel/sts/         # GetCallerIdentity XML
  internal/server/             # HTTP listen, health, pipeline
  docs/                        # this folder
```

The binary directory is lowercase `cmd/noctaxris` on purpose (Unix command name). The Go module path uses PascalCase `Noctaxris` to match the GitHub repo.

## Startup (`cmd/noctaxris`)

1. Load config from env (`config.LoadFromEnv`).
2. Require root access key id and secret (`NOCTAXRIS_ROOT_*`). Fail if missing.
3. Create the data root directory.
4. Load or create a 32-byte master key file (`LoadOrCreateMasterKey`).
5. Open SQLite at `$DATA_ROOT/state.db` and `EnsureRoot` for the injected credentials.
6. Open the audit writer under `$DATA_ROOT/cloudtrail/`.
7. Serve HTTP, or TLS when both cert and key paths are set.

## Request path (`internal/server`)

```text
HTTP request
  ├─ GET /_noctaxris/health  → 200 "ok" (no auth, no audit)
  └─ anything else
       ├─ read body (capped)
       ├─ authn.Verify (SigV4 header or query)
       │    └─ fail → 403 AWS error + audit
       ├─ Action = GetCallerIdentity
       │    └─ XML 200 + audit success (no IAM Evaluate)
       └─ other actions → 501 NotImplemented + audit
```

`sts:GetCallerIdentity` skips IAM Evaluate after SigV4 because AWS documents that this API requires no permissions. The authz package still unit-tests deny/allow for later APIs.

## Store and crypto (`internal/store`)

- `Seal` / `Unseal`: ChaCha20-Poly1305 with a random nonce prepended to ciphertext.
- Access key secrets are stored as ciphertext blobs (SigV4 needs the real secret).
- Decrypt is named `Unseal` so it does not clash with `store.Open`.
- Optional policy tables exist for identity-policy attachments used by authz tests and later IAM APIs.

Driver: `modernc.org/sqlite` (pure Go, `CGO_ENABLED=0` for distroless).

## Audit (`internal/kernel/audit`)

`Writer` appends one JSON object per line to `$dir/events.jsonl`. Fields follow CloudTrail record shapes (`eventVersion` `1.11`, `eventTime`, `errorCode`, `requestID`, and related fields). Secrets must never appear in audit payloads.
