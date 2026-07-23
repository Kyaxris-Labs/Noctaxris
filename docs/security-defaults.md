# Security defaults

These defaults are intentional product posture for a local emulator that people will point SDKs and AWS CLI at.

## Host and container

- Compose publishes only `127.0.0.1:4566` on the host. That is not `0.0.0.0` on the host.
- Inside the container the process listens on `0.0.0.0:4566` so the published mapping works. Compose sets `NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN=1` for that bind only. Do not remap host ports to `0.0.0.0` without TLS.
- Compose does **not** set `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE`. Function URL / HTTP API `NONE` stay off on the container bind unless you opt in (CTF labs): `docker compose -f compose.yaml -f compose.lab-open.yaml up` from `docker/`. Loopback process listen still allows `NONE` without that env.
- No host `docker.sock` mount on the API service. Nested compute uses Compose service `noctaxris-engine` over TLS on the Compose network (`NOCTAXRIS_DOCKER_HOST=tcp://noctaxris-engine:2376`, `NOCTAXRIS_DOCKER_CERT_PATH=/certs/client`). The engine API is not published to the host. Runtime rejects `unix://`, `npipe://`, and any host string containing `docker.sock`. Non-default engine URLs require `NOCTAXRIS_DOCKER_HOST_ALLOWLIST`. TLS client PEMs are required whenever Docker host is set.
- Compose splits volumes: `noctaxris-data` holds API-only state (`master.key`, `state.db`, sealed material, S3 bytes). `noctaxris-compute` mounts at `/var/lib/noctaxris/lambda` on the API (read-write for unpack) and on the engine as `:ro` so DinD can bind-mount Lambda code into tasks without rewriting zip trees. `noctaxris-compute-init` chowns that volume to UID `65532` before the API starts (named volumes are otherwise root-owned). Residual: engine compromise remains a nested-escape class on Docker Desktop hosts.
- Image pulls through DinD are allowlisted (lab registry `127.0.0.1:4566/...`, rewritten `host.docker.internal`, pinned public Lambda bases, and documented lab images such as `alpine:3.20`). Other registries fail closed unless listed in `NOCTAXRIS_IMAGE_PULL_ALLOWLIST` (digest pins required for registry hosts).
- JWT authorizers and AppSync Cognito issuers default to lab Cognito shapes only (in-process JWKS). Arbitrary remote JWKS fetch is disabled. Optional escape hatch: `NOCTAXRIS_ALLOW_REMOTE_JWKS=1` plus `NOCTAXRIS_JWKS_HOST_ALLOWLIST` (public hosts only; no redirects, no RFC1918/link-local/metadata; dial pins to IPs validated at connect time).
- Nested data engines (RDS Postgres, ElastiCache Valkey/Redis, DocumentDB Mongo-compatible, MQ RabbitMQ, OpenSearch) run as labeled containers on the same DinD path. Compose does **not** publish Postgres, Redis/Valkey, Mongo, AMQP, or OpenSearch ports on the host. Without a healthy nested container, MQ/OpenSearch fail closed (`CREATION_FAILED` / `CreateFailed` with `stub://`); never host-publish broker or search ports.
- Lab SQL uses the **RDS Data API** HTTP facade on `:4566`. When DinD has started nested Postgres, ExecuteStatement prefers in-process `pgx` against the nested data-plane DSN only (`noctaxris-data-rds-*`, no host-published ports / no operator DSN), and falls back to `psql` inside that container when the wire dial fails. Without a nested container (or while status is still `creating`), ExecuteStatement returns `DatabaseUnavailableException` (no production SELECT stub). Nested-network endpoint strings on Describe* responses are for DinD-side smoke only, not WAN-reachable listeners.
- When DinD is unset, Create* still records control-plane state and nested start is a no-op (status may stay `creating`). Paths never fall through to host Docker or invent host-published DB ports.
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
- Nested data-plane and ECS networks stay `Internal: true`. ECS / CodeBuild / Batch ExtraHosts host-gateway injection is off by default (`NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1` to opt in).
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
- Inactive access keys are rejected at SigV4 verification.
- For non-`UNSIGNED-PAYLOAD` requests, SigV4 binds `X-Amz-Content-Sha256` to `sha256(body)` (mismatch fails closed). `UNSIGNED-PAYLOAD` is limited to S3 query (presigned) authentication.
- SigV4 requires `host` in `SignedHeaders` (header and query auth). Presigned `X-Amz-Expires` max is 604800.
- AppSync API keys are returned once in plaintext; only an HMAC-SHA256 (master key) hash is stored at rest.

## Auth

Unauthenticated or alternate-auth paths (no SigV4 required):

| Path / API | Auth |
|------------|------|
| `GET /_noctaxris/health` | Open (liveness) |
| `GET /_noctaxris/ready` | Open (readiness; SQLite ping, optional engine TLS dial) |
| `GET /cognito-idp/{region}/{pool}/.well-known/jwks.json` | Public JWKS on the loopback listener |
| `AssumeRoleWithSAML` / `AssumeRoleWithWebIdentity` | Federation token crypto (not SigV4) |
| Lambda Function URL with AuthType `NONE` | Open invoke on `/lambda-url/...` when listen is loopback, or with `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1`. CORS defaults to `Access-Control-Allow-Origin: *` unless `Cors.AllowOrigins` / `NOCTAXRIS_FUNCTION_URL_CORS_ORIGINS` is set |
| HTTP API routes with authorizer `NONE` | Open invoke on `/http-api/...` (same open-data-plane gate; no CORS `*` by default) |
| AppSync GraphQL | `API_KEY`, `AWS_IAM` (SigV4), or Cognito User Pools Bearer JWT |
| S3 anonymous GetObject / HeadObject | Off by default. With `NOCTAXRIS_ALLOW_ANONYMOUS_S3=1`, unsigned path-style object GET/HEAD only when bucket policy Principal `"*"` / `{"AWS":"*"}` Allows `s3:GetObject` or the object canned ACL is `public-read` / `public-read-write`. Explicit Deny beats ACL. List/Put and other S3 APIs stay SigV4-required |

All other AWS API paths require a valid SigV4 signature (header or query) for a known access key.

Non-loopback listen without TLS fails process start unless `NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN=1` (Compose sets this because the container binds `0.0.0.0` while the host publish stays `127.0.0.1:4566`). Prefer TLS (`NOCTAXRIS_TLS_CERT` / `NOCTAXRIS_TLS_KEY`) for any intentional non-loopback exposure. Peer containers on the Compose network can reach the cleartext API even when host publish is loopback-only.

Additional auth notes:

- Temporary credentials require a matching `X-Amz-Security-Token`.
- Presigned S3 GET/PUT use query SigV4 (`X-Amz-Expires` max 604800).
- `GetCallerIdentity` succeeds after SigV4 without an IAM permission check.
- Lab S3, SQS, Lambda, ECR, SNS, Secrets Manager, and DynamoDB dataplane paths use same-account identity **or** resource policy Allow, and cross-account identity **and** resource policy Allow.
- Lab KMS APIs use `EvaluateKMS` (key policy explicit allow or grant, always required, plus identity for cross-account).
- Lab Lambda configure APIs use identity Evaluate plus PassRole/trust.
- Organizations SCP/RCP filters apply on member authorize paths, including OU-path inheritance.
- SNS HTTP(S) subscription endpoints must be the lab catcher on loopback `:4566` (`/_noctaxris/sns-http-catcher`), or an exact URL in `NOCTAXRIS_SNS_HTTP_ALLOWLIST` that resolves to a public host (private, loopback, link-local, and metadata targets are rejected even when listed). Delivery does not follow redirects.
- Cognito management APIs require SigV4 and identity Allow.
- API Gateway JWT routes reject missing, expired, not-yet-valid (`nbf`), or invalid Bearer tokens. IAM routes reject unsigned requests. HTTP API resource policies are not invented. `IdentitySource` must be `$request.header.Authorization`.
- Gateway CredentialsArn requires PassRole plus matching service trust at create, and role-session `lambda:InvokeFunction` evaluation at invoke when set. Without CredentialsArn, HTTP API invoke requires a Lambda resource policy Allow for `apigateway.amazonaws.com`. AppSync Lambda data sources require a resource policy Allow for `appsync.amazonaws.com`. CodeDeploy serviceRoleArn requires PassRole plus matching service trust when set.
- Associated WAFv2 Web ACLs must exist at Associate time. Invoke-time association evaluation errors fail closed (403).
- `iam:PassRole` evaluation populates `iam:PassedToService` from the target service principal.
- Deferred depth returns `501 NotImplemented` or an explicit fail-closed error after successful authn. Never silent Allow.

## Residual escape notes

- Privileged DinD (`noctaxris-engine`) remains the default packaged compute plane and a nested-escape class on Docker Desktop / shared-kernel hosts. Volume split keeps `master.key` off the engine; the compute volume is `:ro` on the engine (API still writes unpack paths). Engine compromise can still reach nested networks and bind-mounted code views. LAN expose of `:4566` with Invoke/RunTask principals is engine-trust equivalent. Restricted-engine overlay is experimental only.
- `host.docker.internal` ExtraHosts is an intentional path from the Lambda function network to the host-published API only when opted in. It is not public internet egress. ECS-path tasks do not get that entry unless `NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1`.
- Go module `github.com/docker/docker` (client SDK) still surfaces several Docker Engine CVEs with Fixed in: N/A on that module path (fixes landed in Engine / `github.com/moby/moby/v2` only). CI allowlists those GO IDs in `scripts/govulncheck-allowlist.txt` so new app vulns still fail the job. Noctaxris never calls `CopyToContainer` / `CopyFromContainer` or `PUT /containers/{id}/archive`; Lambda and related code reach nested containers via read-only bind mounts. AuthZ-plugin bypass findings also do not apply to the packaged engine path (no AuthZ plugins). Residual is still the nested engine binary version and who can talk to it over TLS on the Compose network.

## Optional TLS

Set both `NOCTAXRIS_TLS_CERT` and `NOCTAXRIS_TLS_KEY` to serve TLS. If either is empty, the server uses plain HTTP. Compose and `/_noctaxris/health` / `/_noctaxris/ready` probes are HTTP by default; enabling TLS without updating probe URLs or client endpoint URLs is a common footgun (clients keep calling `http://127.0.0.1:4566` while the listener expects HTTPS). Non-loopback listen without TLS requires `NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN=1`. Example keys under `docker/` or docs are for local labs only; do not reuse them outside the lab.
