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
| `NOCTAXRIS_DOCKER_HOST` | empty | Nested DinD engine URL (Compose sets `tcp://noctaxris-engine:2376`). Empty disables Lambda compute for unit tests. |
| `NOCTAXRIS_DOCKER_CERT_PATH` | empty | Directory with `ca.pem`, `cert.pem`, and `key.pem` for TLS to the engine (Compose sets `/certs/client`). |
| `NOCTAXRIS_LAMBDA_ENDPOINT_URL` | `http://host.docker.internal:4566` when unset in compute | API URL injected into function containers for in-function SDK calls. |

Federation is fail-closed. If these are unset and no IdP rows exist in the store, SAML/OIDC STS APIs deny with AccessDenied / InvalidIdentityToken rather than accepting unsigned tokens.

`NOCTAXRIS_ACCOUNT_ID`, root access key id, OIDC URL/client id, SAML provider name, and SAML metadata path are validated at startup (`internal/validate`). Invalid values fail process start.

## Data layout under `NOCTAXRIS_DATA_ROOT`

| Path | Role |
|------|------|
| `master.key` | 32-byte AEAD key (mode `0600` when created) |
| `state.db` | SQLite accounts, users, keys, policies, roles, KMS, S3 bucket/object metadata, IdP config |
| `s3/` | Object bytes (path-style layout under account and bucket) |
| `cloudtrail/events.jsonl` | Audit trail |

## Docker / Compose

Files live under `docker/`:

- `Dockerfile`: multi-stage build (`golang:1.26-bookworm` → distroless nonroot), `CGO_ENABLED=0`
- `compose.yaml`: publish `127.0.0.1:4566:4566`, named volume for data, `read_only: true`, tmpfs `/tmp`, no `docker.sock`
- `.env.example`: sample root keys for local Compose

Copy `.env.example` to `.env`, set real lab keys, then:

```bash
docker compose -f docker/compose.yaml --env-file docker/.env up --build
curl http://127.0.0.1:4566/_noctaxris/health
```

Root `.dockerignore` keeps `.env`, git metadata, and similar junk out of the build context.
