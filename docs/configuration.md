# Configuration

All settings come from environment variables. Defaults favor a locked-down local lab.

## Environment variables

| Variable | Default | Meaning |
|----------|---------|---------|
| `NOCTAXRIS_LISTEN` | `127.0.0.1:4566` | Bind address (process). Only `localhost`, `127.0.0.0/8`, and `::1` count as loopback. Port-only (`:4566`), empty host, `0.0.0.0`, and `::` are non-loopback (all-interfaces). Compose overrides to `0.0.0.0:4566` inside the container so the published host port works. |
| `NOCTAXRIS_DATA_ROOT` | `/var/lib/noctaxris` | Persistent data directory |
| `NOCTAXRIS_MASTER_KEY_FILE` | sibling `$PARENT/$BASENAME-secrets/master.key` when empty | 32-byte master key path. Must stay outside `NOCTAXRIS_DATA_ROOT` unless `NOCTAXRIS_ALLOW_MASTER_KEY_IN_DATA_ROOT=1`. |
| `NOCTAXRIS_ALLOW_MASTER_KEY_IN_DATA_ROOT` | empty (off) | Set to `1` to allow `master.key` under the data root (historical `$DATA_ROOT/master.key` when `NOCTAXRIS_MASTER_KEY_FILE` is empty). Opt-in only; prefer a path outside the data root. |
| `NOCTAXRIS_TLS_CERT` | empty | PEM cert path (TLS only if both cert and key set) |
| `NOCTAXRIS_TLS_KEY` | empty | PEM key path |
| `NOCTAXRIS_ROOT_ACCESS_KEY_ID` | required | Bootstrap root access key id. The shipped `docker/.env.example` pair is refused when listen is non-loopback (including Compose `0.0.0.0`); generate unique values for Compose. |
| `NOCTAXRIS_ROOT_SECRET_ACCESS_KEY` | required | Bootstrap root secret (encrypted before disk). Same example-root refuse rule as the access key id. |
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
| `NOCTAXRIS_APIGW_HTTP_PROXY` | empty (off) | Set to `1` to allow API Gateway `HTTP_PROXY` / `VPC_LINK` integrations. Default rejects those types. |
| `NOCTAXRIS_APIGW_HTTP_PROXY_ALLOWLIST` | empty | Comma-separated hosts or URL prefixes allowed as `HTTP_PROXY` / `VPC_LINK` IntegrationUri when proxy is on. Link-local, metadata, loopback, and private hosts need an entry that names that host. Fetches do not follow redirects; dial uses a pinned DialContext. |
| `NOCTAXRIS_SNS_HTTP_EGRESS` | empty (off) | Set to `1` to honor `NOCTAXRIS_SNS_HTTP_ALLOWLIST` for SNS HTTP(S) subscriptions beyond the lab catcher. Unset/off: only `127.0.0.1:4566/_noctaxris/sns-http-catcher` (allowlist ignored). |
| `NOCTAXRIS_SNS_HTTP_ALLOWLIST` | empty | Comma-separated exact HTTP(S) URLs allowed beyond the lab catcher when `NOCTAXRIS_SNS_HTTP_EGRESS=1`. Listed URLs still reject private, loopback, link-local, and metadata hosts; delivery does not follow redirects. Ignored when egress is off. |
| `NOCTAXRIS_INJECT_HOST_GATEWAY` | disabled | Set to `1` to inject `host.docker.internal:host-gateway` ExtraHosts on nested Lambda function containers (in-function SDK labs). Prefer `docker/compose.lab-host-gateway.yaml` over changing the code default. |
| `NOCTAXRIS_PUBLISH_ADDR` | `127.0.0.1` | Host address for Compose port publish (`${NOCTAXRIS_PUBLISH_ADDR}:4566:4566`). Use `0.0.0.0` only for short host-gateway lab sessions on Docker Desktop DinD; prefer TLS if exposing beyond loopback. |
| `NOCTAXRIS_INJECT_ECS_HOST_GATEWAY` | disabled | Set to `1` to inject `host.docker.internal:host-gateway` ExtraHosts on nested ECS / CodeBuild / Batch containers (Internal `noctaxris-ecs`). Prefer `docker/compose.lab-ecs-host-gateway.yaml` over changing the code default. Default off. |
| `NOCTAXRIS_NESTED_PORT_PUBLISH` | disabled | Set to `1` so nested data containers bind their listen port on the DinD engine host (`noctaxris-engine`). Prefer `docker/compose.lab-nested-ports.yaml`, which also publishes selected ports to `127.0.0.1` on the operator host (includes Neptune Gremlin `:8182` and Neo4j Bolt `:7687`). Default Compose leaves this off (no host DB/cache/broker ports). Mapping ports on the API service alone cannot reach DinD-internal engines. |
| `NOCTAXRIS_NEPTUNE_ENGINE` | `gremlin` | Nested Neptune graph backend: `gremlin` (default `tinkerpop/gremlin-server`) or `neo4j` (`neo4j:5-community` Bolt `:7687`). Empty defaults to gremlin. Unknown values fail CreateDBCluster closed. Per-cluster override: query `GraphEngine` / `DbType` or tag `noctaxris:neptune-engine`. |
| `NOCTAXRIS_SHARED_KAFKA` | off | Set to `1` / `true` / `on` to bind MSK metadata to the shared DinD Redpanda singleton `noctaxris-lab-kafka` (`noctaxris-lab-kafka:9092`). Unset, empty, `0`, `false`, or `off` keeps per-cluster `noctaxris-msk-<name>` containers. Any other value fails process start. |
| `NOCTAXRIS_SHARED_MQTT` | off | Set to `1` / `true` / `on` to start shared Mosquitto `noctaxris-lab-mqtt` and the API MQTT shadow bridge. Off leaves HTTP shadows only (no MQTT wire). Same strict bool parsing as shared Kafka. |
| `NOCTAXRIS_BROKER_PORT_PUBLISH` | off | Set to `1` so shared Kafka/MQTT `StartDataPlane` may bind `:9092` / `:1883` on the DinD engine for Compose-network dial (`noctaxris-engine:1883` / `:9092`). Narrow gate: does not enable blanket nested DB/cache publish. `docker/compose.lab-brokers.yaml` sets shared Kafka/MQTT and this flag together. |
| `NOCTAXRIS_COMPUTE_RUNTIME` | `dind` | Nested compute path. Only `dind` (or unset) is accepted. Unknown values fail process start. Nested data engines use the same DinD path. |
| `NOCTAXRIS_RDS_DATA_PGX` | prefer on | Set to `0` / `false` / `off` to force RDS Data API nested CLI (skip wire dial for Postgres `pgx` and MySQL/MariaDB drivers). Default prefers wire dial against the nested data-plane DSN only, then falls back to nested `psql` / `mysql`. Transactions still require wire sessions. |
| `NOCTAXRIS_LAMBDA_ENDPOINT_URL` | `http://host.docker.internal:4566` when unset in compute | API URL injected into function containers for in-function SDK calls. |
| `NOCTAXRIS_CLOUDTRAIL_INJECT` | disabled | Set to `1` to enable lab-only `cloudtrail:InjectEvents` and `cloudtrail:InjectInsightsEvents` (`NoctaxrisCloudTrail.*`) for seeding forensic JSONL events. Default off returns AccessDenied. |
| `NOCTAXRIS_CLOUDTRAIL_TRUST_XFF` | disabled | Set to `1` to use the first `X-Forwarded-For` hop for CloudTrail-shaped audit `sourceIPAddress` only when the TCP peer is also covered by `NOCTAXRIS_TRUSTED_PROXIES`. Default uses TCP `RemoteAddr`. Authz `aws:SourceIp` always stays the peer address. |
| `NOCTAXRIS_TRUSTED_PROXIES` | empty | Comma-separated CIDRs (or bare IPs) whose peers may supply `X-Forwarded-For` for WAFv2 SourceIP matching and (with `NOCTAXRIS_CLOUDTRAIL_TRUST_XFF`) audit `sourceIPAddress`. Empty ignores XFF for those paths. |
| `NOCTAXRIS_CLOUDTRAIL_GZIP` | disabled | Set to `1` to gzip-compress CloudTrail trail delivery objects under the AWSLogs hive (`.json.gz`). Athena reads decompress by suffix/magic. |
| `NOCTAXRIS_ATHENA_ENGINE` | `auto` | Athena SQL engine: `auto` prefers DuckDB when `NOCTAXRIS_DUCKDB_URL` or nested DinD DuckDB is available then falls back in-process; `duckdb` fail-closed without engine; `inprocess` skips DuckDB. |
| `NOCTAXRIS_DUCKDB_URL` | empty | Pre-configured DuckDB HTTP base URL (skips nested `noctaxris-lab-duck` ensure). Used by Athena and CUR Parquet emit. |
| `NOCTAXRIS_DUCKDB_IMAGE` | `floci/floci-duck:latest` | Allowlisted nested DuckDB HTTP shim image. |
| `NOCTAXRIS_DUCKDB_S3_ENDPOINT` | `http://host.docker.internal:4566` | Lab S3/API URL as seen from the DuckDB container. |
| `NOCTAXRIS_CUR_EMIT` | on | Set `0` / `off` / `false` to skip CUR S3 artifact writes on Put/Modify. |
| `NOCTAXRIS_GUARDDUTY_INJECT` | disabled | Set to `1` to enable lab-only `guardduty:InjectFindings` (`NoctaxrisGuardDuty.InjectFindings`). Default off returns AccessDenied. |
| `NOCTAXRIS_MACIE_INJECT` | disabled | Set to `1` to enable lab-only `macie2:InjectFindings` (`NoctaxrisMacie.InjectFindings`) including canned S3 object matches. Default off returns AccessDenied. |
| `NOCTAXRIS_VPCFLOW_INJECT` | disabled | Set to `1` to enable lab-only `ec2:InjectFlowLogs` (`NoctaxrisEC2.InjectFlowLogs`). Default off returns AccessDenied. |
| `NOCTAXRIS_LAB_FORENSICS` | disabled | Set to `1` to enable lab FreezeClock/UnfreezeClock/SetClock/BulkSeed (`NoctaxrisLab.*`). Default off returns AccessDenied. |
| `NOCTAXRIS_ROUTE53_QUERY_LOG_INJECT` | disabled | Set to `1` to enable lab Route 53 query log inject to CloudWatch Logs. Default off returns AccessDenied. |
| `NOCTAXRIS_COGNITO_INSECURE_CODES` | disabled | Set to `1` to restore Cognito lab stub confirmation codes (`123456` for forgot/attr-verify; any-non-empty `ConfirmSignUp`). Default off uses high-entropy single-use codes. |

### Cognito confirmation codes (`NOCTAXRIS_COGNITO_INSECURE_CODES`)

Default Cognito forgot-password, sign-up, and attribute-verify confirmation codes are random (8+ hex characters), single-use, and expire after one hour. `ConfirmForgotPassword`, `ConfirmSignUp`, and `VerifyUserAttribute` reject wrong or reused codes with `CodeMismatchException`.

Set `NOCTAXRIS_COGNITO_INSECURE_CODES=1` only for intentional insecure labs: ForgotPassword and attribute verify store fixed `123456`, and `ConfirmSignUp` accepts any non-empty code (previous stub behavior).

### Lab forensics helpers (`NOCTAXRIS_LAB_FORENSICS`)

When `NOCTAXRIS_LAB_FORENSICS=1`, SigV4 service `noctaxris-lab` accepts:

| Action | Behavior |
|--------|----------|
| `FreezeClock` / `UnfreezeClock` | Pin or clear the lab wall clock used for audit timestamps and similar lab time (SigV4 skew checks still use real time) |
| `SetClock` | Set the lab clock to an ISO-8601 / RFC3339 instant |
| `BulkSeed` | Seed a named forensic scenario (`ScenarioId` required: `suspicious-login`, `s3-data-exfil`, `crypto-mining`) into CloudTrail JSONL and optional GuardDuty findings (canned shapes; not live traffic) |

Default off returns AccessDenied. These are Noctaxris lab extensions, not AWS public APIs.

Federation is fail-closed. If these are unset and no IdP rows exist in the store, SAML/OIDC STS APIs deny with AccessDenied / InvalidIdentityToken rather than accepting unsigned tokens.

`NOCTAXRIS_ACCOUNT_ID`, root access key id, OIDC URL/client id, SAML provider name, and SAML metadata path are validated at startup (`internal/validate`). Invalid values fail process start.

## Data layout under `NOCTAXRIS_DATA_ROOT`

| Path | Role |
|------|------|
| `state.db` | SQLite accounts, users, keys, policies, roles, KMS, S3 bucket/object metadata, IdP config, Cognito pools, Gateway APIs, and other lab service rows |
| `s3/` | Object bytes (path-style layout under account and bucket) |
| `cloudtrail/events.jsonl` | Audit trail |
| `bcm-exports/` | BCM Data Exports sample CSV/JSON under account folders |
| `lambda/` | Lambda zip contents shared with DinD via the `noctaxris-compute` Compose volume (not the API-only `noctaxris-data` volume) |
| `transfer/` | Transfer Family SFTP-shaped user sandboxes (`transfer/ACCOUNT/SERVER/home/USER/`) |
| `transcribe/` | Canned Transcribe transcript JSON under account folders |
| `ecr/` | Registry V2 blob/manifest content when the lab registry stores image layers on disk |

### Master key location

The AEAD master key is **not** colocated with ciphertext by default. When `NOCTAXRIS_MASTER_KEY_FILE` is empty, the process uses a sibling path outside the data root (for `NOCTAXRIS_DATA_ROOT=/var/lib/noctaxris`, that is `/var/lib/noctaxris-secrets/master.key`). On load and create, the file mode is enforced to `0600`; a world-readable key is refused after a best-effort `chmod 0600` (Unix hosts; Windows is best-effort ACL/`chmod` only).

Default Compose mounts `noctaxris-secrets` at `/var/lib/noctaxris-secrets` and sets `NOCTAXRIS_MASTER_KEY_FILE` there (writable for UID `65532` under `read_only: true`). To keep the historical `$DATA_ROOT/master.key` layout instead, set `NOCTAXRIS_ALLOW_MASTER_KEY_IN_DATA_ROOT=1` and point `NOCTAXRIS_MASTER_KEY_FILE` at the colocated path. Without that opt-in, any master key path under the data root fails process start. Loss of the master key makes sealed material unrecoverable.

**Single API replica only.** Do not run multiple Noctaxris API processes against one data root. SQLite sets `busy_timeout=5000` on every connection via the DSN. Multi-instance access is unsupported and can corrupt state. WAL is not enabled by default; one API process is the durable-lab posture. SQS receive/send serialize in-process so concurrent claims stay atomic without a global `_txlock=immediate` (which would deadlock nested writers such as CloudFormation → CreateBucket).

## Schema, backup, and upgrades

Schema evolution is additive (`CREATE TABLE IF NOT EXISTS`, `ALTER TABLE ... ADD COLUMN` with duplicate-column ignore). A `schema_version` marker row exists (currently `1`) for operators and tests; migrations remain independent `Ensure*` helpers and do not use that integer as a migration ledger. There is no down-migration.

Operator runbook (stop → tar volumes → restore verify → start, plus image-upgrade notes and the single-replica rule): [ops.md](ops.md).

## Docker / Compose

Files live under `docker/`:

- `Dockerfile`: multi-stage build (`golang:1.26.5-bookworm` → distroless nonroot), `CGO_ENABLED=0`
- `compose.yaml`: publish `${NOCTAXRIS_PUBLISH_ADDR:-127.0.0.1}:4566:4566` (default loopback), `noctaxris-data` for API sealed state, `noctaxris-secrets` for `master.key` (`NOCTAXRIS_MASTER_KEY_FILE`), `noctaxris-compute` for Lambda code (API RW, engine `:ro`), digest-pinned `docker:27-dind` / `busybox` init (chowns compute + secrets to UID `65532`), restricted DinD engine (`privileged: false` + caps/devices + `cgroup: host` + `/sys/fs/cgroup` rw + dockerd `--ipv6=false`), `read_only: true`, tmpfs `/tmp`, no `docker.sock`, no host publish of database/cache/search ports, healthchecks on API and engine. Privileged engine opt-in: `compose.engine-privileged.yaml`. Lab overlays: `compose.lab-open.yaml` (open data plane), `compose.lab-host-gateway.yaml` (Lambda ExtraHosts), `compose.lab-ecs-host-gateway.yaml` (ECS / CodeBuild / Batch ExtraHosts), `compose.lab-brokers.yaml` (shared Kafka/MQTT + `NOCTAXRIS_BROKER_PORT_PUBLISH`), `compose.lab-nested-ports.yaml` (selected nested data TCP on `127.0.0.1` via DinD engine, including `9092`/`1883` when shared brokers run)
- `.env.example`: sample root keys for local Compose

Copy `.env.example` to `.env`, set real lab keys, then:

```bash
docker compose -f docker/compose.yaml --env-file docker/.env up --build
curl http://127.0.0.1:4566/_noctaxris/health
curl http://127.0.0.1:4566/_noctaxris/ready
curl http://127.0.0.1:4566/_noctaxris/version
```

Expect liveness body `ok`, readiness body `ready`, and version body matching `VERSION` (e.g. `1.3.0`). Distroless probes use `/noctaxris healthcheck` inside the API container.

Root `.dockerignore` keeps `.env`, git metadata, and similar junk out of the build context.
