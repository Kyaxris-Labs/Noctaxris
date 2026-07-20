# Configuration

All settings come from environment variables. Defaults favor a locked-down local lab.

## Environment variables

| Variable | Default | Meaning |
|----------|---------|---------|
| `NOCTAXRIS_LISTEN` | `127.0.0.1:4566` | Bind address (process). Compose overrides to `0.0.0.0:4566` inside the container so the published host port works. |
| `NOCTAXRIS_DATA_ROOT` | `/var/lib/noctaxris` | Persistent data directory |
| `NOCTAXRIS_MASTER_KEY_FILE` | `$DATA_ROOT/master.key` when empty | 32-byte master key path |
| `NOCTAXRIS_TLS_CERT` | empty | PEM cert path (TLS only if both cert and key set) |
| `NOCTAXRIS_TLS_KEY` | empty | PEM key path |
| `NOCTAXRIS_ROOT_ACCESS_KEY_ID` | required | Bootstrap root access key id |
| `NOCTAXRIS_ROOT_SECRET_ACCESS_KEY` | required | Bootstrap root secret (encrypted before disk) |
| `NOCTAXRIS_ACCOUNT_ID` | `000000000001` | 12-character account id |
| `NOCTAXRIS_SAML_IDP_METADATA` | empty | Path to SAML IdP metadata XML. When set, seeded into the store at startup for `AssumeRoleWithSAML`. |
| `NOCTAXRIS_SAML_IDP_NAME` | `default` | SAML provider name used when seeding metadata |
| `NOCTAXRIS_OIDC_ISSUER_URL` | empty | OIDC issuer URL. When set with client id, seeded for `AssumeRoleWithWebIdentity`. |
| `NOCTAXRIS_OIDC_CLIENT_ID` | empty | OIDC audience / client id (required if issuer URL is set) |
| `NOCTAXRIS_DOCKER_HOST` | empty | Nested DinD engine URL (Compose sets `tcp://noctaxris-engine:2376`). Empty disables DinD compute for unit tests (Lambda and nested data engines). Rejects `unix://`, `npipe://`, and `docker.sock`. Must be the default engine URL or an entry in `NOCTAXRIS_DOCKER_HOST_ALLOWLIST`. |
| `NOCTAXRIS_DOCKER_CERT_PATH` | empty | Directory with `ca.pem`, `cert.pem`, and `key.pem` for TLS to the engine (Compose sets `/certs/client`). Required whenever `NOCTAXRIS_DOCKER_HOST` is set. |
| `NOCTAXRIS_DOCKER_HOST_ALLOWLIST` | empty | Comma-separated exact `tcp://` URLs allowed in addition to `tcp://noctaxris-engine:2376`. |
| `NOCTAXRIS_IMAGE_PULL_ALLOWLIST` | empty | Comma-separated image reference prefixes allowed beyond the built-in lab pin list. Registry hosts require `@sha256:` digests. |
| `NOCTAXRIS_ALLOW_REMOTE_JWKS` | empty | Set to `1` to allow non-lab JWT issuer JWKS fetch (API Gateway / AppSync / STS web identity). Fail-closed when unset. |
| `NOCTAXRIS_JWKS_HOST_ALLOWLIST` | empty | Comma-separated `host` or `host:port` entries required when remote JWKS is enabled. Private, loopback, link-local, and metadata targets are rejected. |
| `NOCTAXRIS_COMPUTE_RUNTIME` | `dind` | Lambda and ECS compute runtime: `dind` (default) or `microvm` (opt-in). Unknown values fail process start. Nested data engines use the DinD path. |
| `NOCTAXRIS_FIRECRACKER_BIN` | empty | Optional path to the Firecracker binary when `NOCTAXRIS_COMPUTE_RUNTIME=microvm`. If empty, `firecracker` must be on `PATH`. |
| `NOCTAXRIS_LAMBDA_ENDPOINT_URL` | `http://host.docker.internal:4566` when unset in compute | API URL injected into function containers for in-function SDK calls. |

Federation is fail-closed. If these are unset and no IdP rows exist in the store, SAML/OIDC STS APIs deny with AccessDenied / InvalidIdentityToken rather than accepting unsigned tokens.

`NOCTAXRIS_ACCOUNT_ID`, root access key id, OIDC URL/client id, SAML provider name, and SAML metadata path are validated at startup (`internal/validate`). Invalid values fail process start.

## Data layout under `NOCTAXRIS_DATA_ROOT`

| Path | Role |
|------|------|
| `master.key` | 32-byte AEAD key (mode `0600` when created). Loss of this key makes sealed material unrecoverable. |
| `state.db` | SQLite accounts, users, keys, policies, roles, KMS, S3 bucket/object metadata, IdP config, Cognito pools, Gateway APIs, and other lab service rows |
| `s3/` | Object bytes (path-style layout under account and bucket) |
| `cloudtrail/events.jsonl` | Audit trail |
| `bcm-exports/` | BCM Data Exports sample CSV/JSON under account folders |
| `lambda/` | Lambda zip contents shared with DinD via the `noctaxris-compute` Compose volume (not the API-only `noctaxris-data` volume) |
| `transfer/` | Transfer Family SFTP-shaped user sandboxes (`transfer/ACCOUNT/SERVER/home/USER/`) |
| `transcribe/` | Canned Transcribe transcript JSON under account folders |
| `ecr/` | Registry V2 blob/manifest content when the lab registry stores image layers on disk |

**Single API replica only.** Do not run multiple Noctaxris API processes against one data root. SQLite sets `busy_timeout=5000` on every connection via the DSN. Multi-instance access is unsupported and can corrupt state. WAL is not enabled by default; one API process is the durable-lab posture. SQS receive/send serialize in-process so concurrent claims stay atomic without a global `_txlock=immediate` (which would deadlock nested writers such as CloudFormation → CreateBucket).

## Schema and upgrades

Schema evolution is additive (`CREATE TABLE IF NOT EXISTS`, `ALTER TABLE ... ADD COLUMN` with duplicate-column ignore). A `schema_version` row is maintained for operators and tests. There is no down-migration. Prefer stop → backup → start on image upgrades.

## Backup and restore

1. Stop Compose (`docker compose -f docker/compose.yaml --env-file docker/.env down`).
2. Archive the data volume (or data root), at least: `master.key`, `state.db`, `s3/`, `lambda/`, `cloudtrail/`, plus `transfer/`, `transcribe/`, `bcm-exports/`, and `ecr/` if used.
3. Restore onto a fresh volume or host path, verify files are present, then start Compose.
4. Without `master.key`, sealed secrets and CMK material cannot be decrypted even if `state.db` is restored.

Example (named volumes `noctaxris-data` and `noctaxris-compute`):

```bash
docker compose -f docker/compose.yaml --env-file docker/.env down
docker run --rm -v noctaxris-data:/data -v "$PWD:/backup" busybox \
  tar czf /backup/noctaxris-data.tgz -C /data .
docker run --rm -v noctaxris-compute:/data -v "$PWD:/backup" busybox \
  tar czf /backup/noctaxris-compute.tgz -C /data .
# restore: extract into empty volumes, then compose up
```

## Docker / Compose

Files live under `docker/`:

- `Dockerfile`: multi-stage build (`golang:1.26.5-bookworm` → distroless nonroot), `CGO_ENABLED=0`
- `compose.yaml`: publish `127.0.0.1:4566:4566` only, `noctaxris-data` for API state, `noctaxris-compute` for Lambda code shared with DinD, `read_only: true`, tmpfs `/tmp`, no `docker.sock`, no host publish of database/cache/search ports, healthchecks on API and engine
- `.env.example`: sample root keys for local Compose

Copy `.env.example` to `.env`, set real lab keys, then:

```bash
docker compose -f docker/compose.yaml --env-file docker/.env up --build
curl http://127.0.0.1:4566/_noctaxris/health
curl http://127.0.0.1:4566/_noctaxris/ready
```

Expect liveness body `ok` and readiness body `ready`. Distroless probes use `/noctaxris healthcheck` inside the API container.

Root `.dockerignore` keeps `.env`, git metadata, and similar junk out of the build context.
