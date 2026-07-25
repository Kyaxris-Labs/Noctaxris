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
| `NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN` | empty | Set to `1` to allow non-loopback bind without TLS. Compose sets this for the container `0.0.0.0` bind while host publish stays `127.0.0.1:4566`. Prefer TLS for any intentional non-loopback exposure. |
| `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE` | empty | Set to `1` to allow Function URL / HTTP API `NONE` when listen is non-loopback (Compose leaves this unset; use `docker/compose.lab-open.yaml` overlay). Loopback listen allows `NONE` without this env. |
| `NOCTAXRIS_ALLOW_ANONYMOUS_S3` | empty | Set to `1` to allow unsigned path-style `GetObject` / `HeadObject` when a bucket policy Principal `"*"` / `{"AWS":"*"}` Allow or object canned ACL `public-read` grants the object. Default Compose leaves this unset. Does not open List/Put or other S3 APIs. |
| `NOCTAXRIS_FUNCTION_URL_CORS_ORIGINS` | empty | Comma-separated CORS AllowOrigins for Function URL `NONE` when `Cors.AllowOrigins` is omitted. Empty keeps lab default `*`. |
| `NOCTAXRIS_HTTP_API_ALLOW_SET_COOKIE` | empty | Set to `1` to pass `Set-Cookie` from HTTP API Lambda proxy responses (stripped by default with hop-by-hop headers). |
| `NOCTAXRIS_SNS_HTTP_ALLOWLIST` | empty | Comma-separated exact HTTP(S) URLs allowed for SNS subscriptions beyond the lab catcher on `127.0.0.1:4566/_noctaxris/sns-http-catcher`. Listed URLs still reject private, loopback, link-local, and metadata hosts; delivery does not follow redirects. |
| `NOCTAXRIS_INJECT_HOST_GATEWAY` | disabled | Set to `1` to inject `host.docker.internal:host-gateway` ExtraHosts on nested Lambda function containers (in-function SDK labs). Prefer `docker/compose.lab-host-gateway.yaml` over changing the code default. |
| `NOCTAXRIS_PUBLISH_ADDR` | `127.0.0.1` | Host address for Compose port publish (`${NOCTAXRIS_PUBLISH_ADDR}:4566:4566`). Use `0.0.0.0` only for short host-gateway lab sessions on Docker Desktop DinD; prefer TLS if exposing beyond loopback. |
| `NOCTAXRIS_INJECT_ECS_HOST_GATEWAY` | disabled | Set to `1` to inject `host.docker.internal:host-gateway` ExtraHosts on nested ECS / CodeBuild / Batch containers (Internal `noctaxris-ecs`). Prefer `docker/compose.lab-ecs-host-gateway.yaml` over changing the code default. Default off. |
| `NOCTAXRIS_COMPUTE_RUNTIME` | `dind` | Nested compute path. Only `dind` (or unset) is accepted. Unknown values fail process start. Nested data engines use the same DinD path. |
| `NOCTAXRIS_RDS_DATA_PGX` | prefer on | Set to `0` / `false` / `off` to force RDS Data API nested-psql (skip `pgx` dial). Default prefers `pgx` against the nested data-plane DSN only, then falls back to nested-psql. |
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

## Schema, backup, and upgrades

Schema evolution is additive (`CREATE TABLE IF NOT EXISTS`, `ALTER TABLE ... ADD COLUMN` with duplicate-column ignore). A `schema_version` marker row exists (currently `1`) for operators and tests; migrations remain independent `Ensure*` helpers and do not use that integer as a migration ledger. There is no down-migration.

Operator runbook (stop → tar volumes → restore verify → start, plus image-upgrade notes and the single-replica rule): [ops.md](ops.md).

## Docker / Compose

Files live under `docker/`:

- `Dockerfile`: multi-stage build (`golang:1.26.5-bookworm` → distroless nonroot), `CGO_ENABLED=0`
- `compose.yaml`: publish `${NOCTAXRIS_PUBLISH_ADDR:-127.0.0.1}:4566:4566` (default loopback), `noctaxris-data` for API state, `noctaxris-compute` for Lambda code (API RW, engine `:ro`), digest-pinned `docker:27-dind` / `busybox` init, restricted DinD engine (`privileged: false` + caps/devices + `cgroup: host` + `/sys/fs/cgroup` rw + dockerd `--ipv6=false`), `read_only: true`, tmpfs `/tmp`, no `docker.sock`, no host publish of database/cache/search ports, healthchecks on API and engine. Privileged engine opt-in: `compose.engine-privileged.yaml`. Lab overlays: `compose.lab-open.yaml` (open data plane), `compose.lab-host-gateway.yaml` (Lambda ExtraHosts), `compose.lab-ecs-host-gateway.yaml` (ECS / CodeBuild / Batch ExtraHosts)
- `.env.example`: sample root keys for local Compose

Copy `.env.example` to `.env`, set real lab keys, then:

```bash
docker compose -f docker/compose.yaml --env-file docker/.env up --build
curl http://127.0.0.1:4566/_noctaxris/health
curl http://127.0.0.1:4566/_noctaxris/ready
curl http://127.0.0.1:4566/_noctaxris/version
```

Expect liveness body `ok`, readiness body `ready`, and version body matching `VERSION` (e.g. `1.1.1`). Distroless probes use `/noctaxris healthcheck` inside the API container.

Root `.dockerignore` keeps `.env`, git metadata, and similar junk out of the build context.
