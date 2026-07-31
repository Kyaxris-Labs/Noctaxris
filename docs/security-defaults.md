# Security defaults

These defaults are intentional product posture for a local emulator that people will point SDKs and AWS CLI at.

## Host and container

- Compose publishes only `127.0.0.1:4566` on the host. That is not `0.0.0.0` on the host.
- Inside the container the process listens on `0.0.0.0:4566` so the published mapping works. Compose sets `NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN=1` for that bind only. Do not remap host ports to `0.0.0.0` without TLS.
- Compose does **not** set `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE`. Function URL / HTTP API `NONE` stay off on the container bind unless you opt in (CTF labs): `docker compose -f compose.yaml -f compose.lab-open.yaml up` from `docker/`. Loopback process listen still allows `NONE` without that env.
- No host `docker.sock` mount on the API service. Nested compute uses Compose service `noctaxris-engine` over TLS on the Compose network (`NOCTAXRIS_DOCKER_HOST=tcp://noctaxris-engine:2376`, `NOCTAXRIS_DOCKER_CERT_PATH=/certs/client`). The engine API is not published to the host. Runtime rejects `unix://`, `npipe://`, and any host string containing `docker.sock`. Non-default engine URLs require `NOCTAXRIS_DOCKER_HOST_ALLOWLIST`. TLS client PEMs are required whenever Docker host is set.
- Compose splits volumes: `noctaxris-data` holds API sealed state (`state.db`, sealed material, S3 bytes). `noctaxris-secrets` mounts at `/var/lib/noctaxris-secrets` with `NOCTAXRIS_MASTER_KEY_FILE=/var/lib/noctaxris-secrets/master.key` so the master key stays outside the data root under a read-only rootfs. `noctaxris-compute` mounts at `/var/lib/noctaxris/lambda` on the API (read-write for unpack) and on the engine as `:ro` so DinD can bind-mount Lambda code into tasks without rewriting zip trees. `noctaxris-compute-init` chowns compute and secrets volumes to UID `65532` before the API starts (named volumes are otherwise root-owned). Neither data nor secrets mounts on the engine. Residual: engine compromise remains a nested-escape class on Docker Desktop hosts.
- Image pulls through DinD are allowlisted (lab registry `127.0.0.1:4566/...`, rewritten `host.docker.internal`, pinned public Lambda bases, and documented lab images such as `alpine:3.20`). Other registries fail closed unless listed in `NOCTAXRIS_IMAGE_PULL_ALLOWLIST` (digest pins required for registry hosts).
- JWT authorizers and AppSync Cognito issuers default to lab Cognito shapes only (in-process JWKS). Arbitrary remote JWKS fetch is disabled. Optional escape hatch: `NOCTAXRIS_ALLOW_REMOTE_JWKS=1` plus `NOCTAXRIS_JWKS_HOST_ALLOWLIST` (public hosts only; no redirects, no RFC1918/link-local/metadata; dial pins to IPs validated at connect time).
- Nested data engines (RDS Postgres/MySQL/MariaDB, ElastiCache/MemoryDB Valkey/Redis, DocumentDB Mongo-compatible, Neptune Gremlin Server or opt-in Neo4j, MQ RabbitMQ, OpenSearch, MSK Redpanda, and optional shared `noctaxris-lab-kafka` / `noctaxris-lab-mqtt`) run as labeled containers on Internal `noctaxris-data` via the same DinD path. Default Compose does **not** publish Postgres, Redis/Valkey, Mongo, Gremlin, Bolt, AMQP, OpenSearch, Kafka, MQTT, or other nested data ports on the host. Without a healthy nested container, MQ/OpenSearch/MSK fail closed (`CREATION_FAILED` / `CreateFailed` / `FAILED` with `stub://` where applicable). Opt-in loopback publish uses `NOCTAXRIS_NESTED_PORT_PUBLISH=1` plus `docker/compose.lab-nested-ports.yaml` (engine-side PortBindings, then `127.0.0.1` maps on `noctaxris-engine`). Shared MSK/MQTT may use the narrower `NOCTAXRIS_BROKER_PORT_PUBLISH=1` gate (`compose.lab-brokers.yaml`) so the API can dial `noctaxris-engine:1883` / `:9092` without opening unrelated DB ports. Residual: any peer on the Compose network can reach those engine-bound broker ports when publish is on. Do not map those ports on the API service; DinD-internal listeners are unreachable that way.
- Shared Mosquitto (when `NOCTAXRIS_SHARED_MQTT=1`) requires lab IoT CA client certificates (`require_certificate`, `allow_anonymous false`); no default MQTT password path. IoT device policies are evaluated fail-closed on Connect/Publish/Subscribe/Receive. Shared Kafka is not multi-tenant across accounts (one MSK metadata row per process when shared mode is on).
- Lab SQL uses the **RDS Data API** HTTP facade on `:4566`. When DinD has started a nested engine, ExecuteStatement dials the nested data-plane DSN only (`noctaxris-data-rds-*`, no host-published ports / no operator DSN by default): Postgres prefers in-process `pgx` then nested `psql`; MySQL/MariaDB prefer `go-sql-driver/mysql` then nested `mysql` CLI. Without a nested container (or while status is still `creating`), ExecuteStatement returns `DatabaseUnavailableException` (no production SELECT stub). Nested-network endpoint strings on Describe* responses are for DinD-side smoke only unless the nested-ports overlay is active.
- When DinD is unset, Create* still records control-plane state and nested start is a no-op (status may stay `creating`). Paths never fall through to host Docker or invent host-published DB ports without the nested-ports opt-in.
- Lambda and ECS compute is nested DinD only (`NOCTAXRIS_COMPUTE_RUNTIME` unset or `dind`). Live Invoke/RunTask require a healthy engine. The path never mounts host Docker.
- Default Compose runs `noctaxris-engine` as restricted DinD: `privileged: false`, explicit `cap_add` / devices / security_opt (`systempaths=unconfined`), `cgroup: host`, a read-write `/sys/fs/cgroup` mount, and dockerd `--ipv6=false` / `--ip6tables=false` so nested containers can start on cgroup v2 without full privileged. Privilege stays inside that nested engine. The API container remains distroless `nonroot` without a host socket. Residual: engine compromise remains a nested-escape class on Docker Desktop / shared-kernel hosts (SYS_ADMIN and host cgroup namespace are still required for classic DinD). If nested containers fail to start on your host, opt in to full privileged DinD: `docker compose -f compose.yaml -f compose.engine-privileged.yaml up` from `docker/`.
- Image runs as distroless `nonroot`. Data dir is seeded owned by UID `65532` so the volume is writable.
- Compose sets `read_only: true` with `/tmp` as tmpfs on the API service.

## Lambda PassRole and trust

- CreateFunction and role-changing UpdateFunctionConfiguration require `iam:PassRole` on the target role plus a trust policy that Allows `sts:AssumeRole` for `lambda.amazonaws.com`.
- Missing PassRole or a trust policy that only names an AWS principal (and not the Lambda service) is Deny.
- Sync Invoke mints temporary credentials for the function role and injects them into the nested container. Same-account callers need identity Allow for `lambda:InvokeFunction` on the function ARN, or a function resource policy that Allows invoke. Cross-account Invoke requires both identity and function policy Allow.

## Platform egress deny vs AWS default internet

- Function containers join DinD network `noctaxris-fn` as a bridge with IP masquerade disabled (not `Internal: true`). WAN SNAT stays off. Host-gateway ExtraHosts is opt-in (`NOCTAXRIS_INJECT_HOST_GATEWAY=1`, or `docker/compose.lab-host-gateway.yaml`) so in-function SDK labs can reach the published API.
- Nested data-plane and ECS networks stay `Internal: true`. ECS / CodeBuild / Batch ExtraHosts host-gateway injection is off by default (`NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1` to opt in). Nested data containers use `CapDrop: ALL` plus a minimal bootstrap `CapAdd` (`CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `SETGID`, `SETUID`) so official Postgres/Mongo entrypoints can chown and drop privileges. Host DB ports stay unpublished unless `NOCTAXRIS_NESTED_PORT_PUBLISH=1` with `compose.lab-nested-ports.yaml` (loopback on the engine hop only).
- On AWS Lambda, functions have internet egress by default unless you attach a VPC without outbound NAT. Lab functions here cannot phone home to the public internet by default.
- Reaching the Noctaxris API from inside a Lambda function uses `host.docker.internal` / host-gateway (`NOCTAXRIS_LAMBDA_ENDPOINT_URL`) only when `NOCTAXRIS_INJECT_HOST_GATEWAY=1`. That path is for the published lab API, not open internet egress, and is not port-scoped. Default Compose publish stays `127.0.0.1`; Docker Desktop DinD labs that need host-gateway may set `NOCTAXRIS_PUBLISH_ADDR=0.0.0.0` for that session only.
- Reusing an existing `noctaxris-fn` with masquerade enabled or legacy `Internal: true` fails closed (or replaces the unused network). Data-plane Internal reuse still refuses non-Internal networks.
- Nested data Create with DinD configured marks status `failed` when container start fails (not silent `creating`). Without DinD, Create may stay `creating` (documented control-plane only).
- CodeBuild / Batch wait on container exit: non-zero exit code becomes Failed. CodePipeline CodeBuild actions with empty DockerHost record Failed (not silent Succeeded).

## Credentials and crypto

- Injected env root credentials become the initial account root.
- Access-key secrets, KMS CMK material, and SSE-S3 DEKs are sealed with ChaCha20-Poly1305 before landing in SQLite.
- Object ciphertext for SSE lives under the data volume filesystem.
- Tests assert plaintext secrets and CMK bytes do not appear in `state.db`.
- Audit events must not carry secret or plaintext key material.
- Inactive access keys are rejected at SigV4 verification. An in-process cache may reuse unsealed access-key material after Lookup; Status, ExpiresAt, and session token are still checked on every Verify, and Delete/UpdateAccessKey invalidate the entry. Allow/Deny decisions and policy document sets are not cached.
- For non-`UNSIGNED-PAYLOAD` requests, SigV4 binds `X-Amz-Content-Sha256` to `sha256(body)` (mismatch fails closed). `UNSIGNED-PAYLOAD` is limited to S3 query (presigned) authentication.
- SigV4 requires `host` in `SignedHeaders` (header and query auth). Presigned `X-Amz-Expires` max is 604800.
- AppSync API keys are returned once in plaintext; only an HMAC-SHA256 (master key) hash is stored at rest.

## Auth

Unauthenticated or alternate-auth paths (no SigV4 required):

| Path / API | Auth |
|------------|------|
| `GET /_noctaxris/health` | Open (liveness) |
| `GET /_noctaxris/ready` | Open (readiness; SQLite ping, optional engine TLS dial) |
| `GET /_noctaxris/version` | Open (product semver, plain text) |
| `GET /cognito-idp/{region}/{pool}/.well-known/jwks.json` | Public JWKS on the loopback listener |
| `AssumeRoleWithSAML` / `AssumeRoleWithWebIdentity` | Federation token crypto (not SigV4) |
| Lambda Function URL with AuthType `NONE` | Open invoke on `/lambda-url/...` when listen is loopback, or with `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1`. CORS defaults to `Access-Control-Allow-Origin: *` unless `Cors.AllowOrigins` / `NOCTAXRIS_FUNCTION_URL_CORS_ORIGINS` is set |
| HTTP API routes with authorizer `NONE` | Open invoke on `/http-api/...` (same open-data-plane gate). CORS is off until `CorsConfiguration` is set on the API; there is no default `AllowOrigins: *` |
| AppSync GraphQL | `API_KEY`, `AWS_IAM` (SigV4), or Cognito User Pools Bearer JWT |
| S3 anonymous GetObject / HeadObject | Off by default. With `NOCTAXRIS_ALLOW_ANONYMOUS_S3=1`, unsigned path-style object GET/HEAD only when bucket policy Principal `"*"` / `{"AWS":"*"}` Allows `s3:GetObject` or the object canned ACL is `public-read` / `public-read-write`. Explicit Deny beats ACL. List/Put and other S3 APIs stay SigV4-required |
| Cognito `InitiateAuth` | Public IdP API (unsigned AWS CLI / SDK shape). Pool/client CRUD and `Admin*` stay SigV4 |

All other AWS API paths require a valid SigV4 signature (header or query) for a known access key.

Non-loopback listen without TLS fails process start unless `NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN=1` (Compose sets this because the container binds `0.0.0.0` while the host publish stays `127.0.0.1:4566`). Prefer TLS (`NOCTAXRIS_TLS_CERT` / `NOCTAXRIS_TLS_KEY`) for any intentional non-loopback exposure. Peer containers on the Compose network can reach the cleartext API even when host publish is loopback-only.

The shipped `docker/.env.example` root pair is allowed only when process listen is loopback. Startup refuses that pair on non-loopback listen (including default Compose `0.0.0.0` and `docker run` with the same bind). Copy `.env.example` to `.env` (or pass `-e` roots) and set unique lab credentials before bringing the API up; see [configuration](configuration.md) and [ops](ops.md).

Additional auth notes:

- Temporary credentials require a matching `X-Amz-Security-Token`.
- Presigned S3 GET/PUT use query SigV4 (`X-Amz-Expires` max 604800).
- `GetCallerIdentity` succeeds after SigV4 without an IAM permission check.
- Lab S3, SQS, Lambda, ECR, SNS, Secrets Manager, and DynamoDB dataplane paths use same-account identity **or** resource policy Allow, and cross-account identity **and** resource policy Allow.
- Lab KMS APIs use `EvaluateKMS` (key policy explicit allow or grant, always required, plus identity for cross-account).
- Lab Lambda configure APIs use identity Evaluate plus PassRole/trust.
- Organizations SCP/RCP filters apply on member authorize paths, including OU-path inheritance.
- SNS HTTP(S) subscription endpoints default to the lab catcher on loopback `:4566` (`/_noctaxris/sns-http-catcher`) only. Exact URLs in `NOCTAXRIS_SNS_HTTP_ALLOWLIST` require `NOCTAXRIS_SNS_HTTP_EGRESS=1` and must resolve to a public host (private, loopback, link-local, and metadata targets are rejected even when listed). Without egress, the allowlist is ignored. Delivery does not follow redirects.
- API Gateway `HTTP_PROXY` / `VPC_LINK` integrations are denied unless `NOCTAXRIS_APIGW_HTTP_PROXY=1` and the IntegrationUri matches `NOCTAXRIS_APIGW_HTTP_PROXY_ALLOWLIST`. Link-local, metadata, loopback, and private hosts require an allowlist entry that names that host. Fetches do not follow redirects; dial uses a pinned DialContext.
- Cognito management APIs (pool/client CRUD, `Admin*`) require SigV4 and identity Allow. `InitiateAuth` is the public IdP exception (unsigned).
- API Gateway JWT routes reject missing, expired, not-yet-valid (`nbf`), or invalid Bearer tokens. IAM routes reject unsigned requests. HTTP API resource policies are not invented. `IdentitySource` must be `$request.header.Authorization`.
- Gateway CredentialsArn requires PassRole plus matching service trust at create, and role-session `lambda:InvokeFunction` evaluation at invoke when set. Without CredentialsArn, HTTP API invoke requires a Lambda resource policy Allow for `apigateway.amazonaws.com`. AppSync Lambda data sources require a resource policy Allow for `appsync.amazonaws.com`. CodeDeploy serviceRoleArn requires PassRole plus matching service trust when set.
- S3 bucket notifications and EventBridge target delivery re-check destination resource policies on emit (`s3.amazonaws.com` / `events.amazonaws.com` + `aws:SourceArn` / `aws:SourceAccount`). Empty notification config is off. EventBridge `PutTargets` without `RoleArn` delivers to SQS/Lambda/SNS/Logs/Kinesis/SFN only when the destination resource policy Allows `events.amazonaws.com` (or account root); empty policy skips that target.
- Associated WAFv2 Web ACLs must exist at Associate time. Invoke-time association evaluation errors fail closed (403).
- `iam:PassRole` evaluation populates `iam:PassedToService` from the target service principal. Configure-time PassRole sets trust `aws:SourceArn` (and `aws:SourceAccount`) for Lambda, EventBridge PutTargets, ECS task definitions, Scheduler schedules, Pipes, Secrets Manager Lambda rotate, API Gateway HTTP `CredentialsArn` / `AuthorizerCredentialsArn`, and Cognito user-pool trigger `RoleArn`.
- Deferred depth returns `501 NotImplemented` or an explicit fail-closed error after successful authn. Never silent Allow.

## Residual escape notes

- Accepted residual: classic DinD on a shared kernel needs `SYS_ADMIN` and host cgroup access even when Compose sets `privileged: false`. That is intentional product posture (no Firecracker / microVM path). Nested-engine compromise remains an escape class on Docker Desktop / shared-kernel hosts. Volume split keeps `noctaxris-data` and `noctaxris-secrets` (`master.key`) off the engine; the compute volume is `:ro` on the engine (API still writes unpack paths). Engine compromise can still reach nested networks and bind-mounted code views. LAN expose of `:4566` with Invoke/RunTask principals is engine-trust equivalent.
- Full privileged DinD (`compose.engine-privileged.yaml`) is opt-in only for hosts that cannot start nested containers. Keep host publish on `127.0.0.1:4566` with that overlay. The API process cannot see Compose `privileged:` and does not refuse privileged engine + non-loopback host publish at startup; operators must not widen `NOCTAXRIS_PUBLISH_ADDR` while the privileged overlay is in use.
- `host.docker.internal` ExtraHosts is an intentional path from the Lambda function network to the host-published API only when opted in. It is not public internet egress. ECS-path tasks do not get that entry unless `NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1`.
- Go module `github.com/docker/docker` (client SDK) still surfaces several Docker Engine CVEs with Fixed in: N/A on that module path (fixes landed in Engine / `github.com/moby/moby/v2` only). CI allowlists those GO IDs in `scripts/govulncheck-allowlist.txt` so new app vulns still fail the job. `CopyToContainer` / `CopyFromContainer` are limited to CodeBuild workspace paths under `/codebuild/src` via `internal/compute` helpers; Lambda and related code still reach nested containers via read-only bind mounts. AuthZ-plugin bypass findings also do not apply to the packaged engine path (no AuthZ plugins). Residual is still the nested engine binary version and who can talk to it over TLS on the Compose network.
- API image Dockerfile bases (`golang` build stage and distroless runtime) are digest-pinned. CI installs `govulncheck` at a pinned module version (not `@latest`) and uploads a Syft SPDX SBOM artifact for the built API image. Cosign image signing is not published in this cut. The smoke job still installs AWS CLI v2 from Amazon’s zip without a pinned checksum (unsigned install residual).
- `NOCTAXRIS_TRUSTED_PROXIES` (comma-separated CIDRs) gates use of `X-Forwarded-For` for WAFv2 SourceIP matching. When empty (default), WAF enforcement uses the TCP peer only. Opt-in CloudTrail audit XFF remains `NOCTAXRIS_CLOUDTRAIL_TRUST_XFF=1` (lab enrichment; do not enable on untrusted multi-hop edges without a trusted proxy layer).

## Optional TLS

Set both `NOCTAXRIS_TLS_CERT` and `NOCTAXRIS_TLS_KEY` to serve TLS. If either is empty, the server uses plain HTTP. Compose and `/_noctaxris/health` / `/_noctaxris/ready` probes are HTTP by default; enabling TLS without updating probe URLs or client endpoint URLs is a common footgun (clients keep calling `http://127.0.0.1:4566` while the listener expects HTTPS). Non-loopback listen without TLS requires `NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN=1`. Example keys under `docker/` or docs are for local labs only; do not reuse them outside the lab.
