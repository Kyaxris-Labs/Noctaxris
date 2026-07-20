# Security defaults

These defaults are intentional product posture for a local emulator that people will point SDKs and AWS CLI at.

## Host and container

- Compose publishes only `127.0.0.1:4566` on the host. That is not `0.0.0.0` on the host.
- Inside the container the process listens on `0.0.0.0:4566` so the published mapping works.
- No host `docker.sock` mount on the API service. Nested compute uses Compose service `noctaxris-engine` over TLS on the Compose network (`NOCTAXRIS_DOCKER_HOST=tcp://noctaxris-engine:2376`, `NOCTAXRIS_DOCKER_CERT_PATH=/certs/client`). The engine API is not published to the host. Runtime rejects `unix://`, `npipe://`, and any host string containing `docker.sock`. Non-default engine URLs require `NOCTAXRIS_DOCKER_HOST_ALLOWLIST`. TLS client PEMs are required whenever Docker host is set.
- Compose splits volumes: `noctaxris-data` holds API-only state (`master.key`, `state.db`, sealed material, S3 bytes). `noctaxris-compute` mounts at `/var/lib/noctaxris/lambda` on both API and engine so DinD can bind Lambda code without reading the sealed API volume. Residual: engine compromise remains a nested-escape class on Docker Desktop hosts.
- Image pulls through DinD are allowlisted (lab registry `127.0.0.1:4566/...`, rewritten `host.docker.internal`, pinned public Lambda bases, and documented lab images such as `alpine:3.20`). Other registries fail closed unless listed in `NOCTAXRIS_IMAGE_PULL_ALLOWLIST` (digest pins required for registry hosts).
- JWT authorizers and AppSync Cognito issuers default to lab Cognito shapes only (in-process JWKS). Arbitrary remote JWKS fetch is disabled. Optional escape hatch: `NOCTAXRIS_ALLOW_REMOTE_JWKS=1` plus `NOCTAXRIS_JWKS_HOST_ALLOWLIST` (public hosts only; no redirects, no RFC1918/link-local/metadata).
- Nested data engines (RDS Postgres, ElastiCache Valkey/Redis, DocumentDB Mongo-compatible) run as labeled containers on the same DinD path. Compose does **not** publish Postgres, Redis/Valkey, Mongo, or OpenSearch ports on the host.
- Lab SQL uses the **RDS Data API** HTTP facade on `:4566`. The default executor is a recorded-statement stub (validates ARNs and authz without Docker; responses include an explicit stub marker). Live Postgres wire protocol from the API process is not enabled. Nested-network endpoint strings on Describe* responses are for DinD-side smoke only, not WAN-reachable listeners.
- When DinD is unset, Create* still records control-plane state and nested start is a no-op (status may stay `creating`). Paths never fall through to host Docker or invent host-published DB ports.
- Default Lambda and ECS runtime is DinD. Opt-in `NOCTAXRIS_COMPUTE_RUNTIME=microvm` is supported only on Linux with usable KVM and a Firecracker binary. WSL2 is DinD-only. Opt-in never mounts host Docker or silently weakens the path.
- `noctaxris-engine` runs privileged DinD so function containers can start. Privilege stays inside that nested engine. The API container remains distroless `nonroot` without a host socket.
- Image runs as distroless `nonroot`. Data dir is seeded owned by UID `65532` so the volume is writable.
- Compose sets `read_only: true` with `/tmp` as tmpfs on the API service.

## Lambda PassRole and trust

- CreateFunction and role-changing UpdateFunctionConfiguration require `iam:PassRole` on the target role plus a trust policy that Allows `sts:AssumeRole` for `lambda.amazonaws.com`.
- Missing PassRole or a trust policy that only names an AWS principal (and not the Lambda service) is Deny.
- Sync Invoke mints temporary credentials for the function role and injects them into the nested container. Same-account callers need identity Allow for `lambda:InvokeFunction` on the function ARN, or a function resource policy that Allows invoke. Cross-account Invoke requires both identity and function policy Allow.

## Platform egress deny vs AWS default internet

- Function containers join DinD network `noctaxris-fn` with `Internal: true`. That is a platform egress deny: no default route to the public internet.
- On AWS Lambda, functions have internet egress by default unless you attach a VPC without outbound NAT. Lab functions here cannot phone home to the public internet by default.
- Reaching the Noctaxris API from inside a function still uses `host.docker.internal` / host-gateway (`NOCTAXRIS_LAMBDA_ENDPOINT_URL`). That path is for the published loopback API, not open internet egress.

## Credentials and crypto

- Injected env root credentials become the initial account root.
- Access-key secrets, KMS CMK material, and SSE-S3 DEKs are sealed with ChaCha20-Poly1305 before landing in SQLite.
- Object ciphertext for SSE lives under the data volume filesystem.
- Tests assert plaintext secrets and CMK bytes do not appear in `state.db`.
- Audit events must not carry secret or plaintext key material.
- Inactive access keys are rejected at SigV4 verification.
- For non-`UNSIGNED-PAYLOAD` requests, SigV4 binds `X-Amz-Content-Sha256` to `sha256(body)` (mismatch fails closed). `UNSIGNED-PAYLOAD` is limited to S3 query (presigned) authentication.

## Auth

Unauthenticated or alternate-auth paths (no SigV4 required):

| Path / API | Auth |
|------------|------|
| `GET /_noctaxris/health` | Open (liveness) |
| `GET /_noctaxris/ready` | Open (readiness; SQLite ping, optional engine TLS dial) |
| `GET /cognito-idp/{region}/{pool}/.well-known/jwks.json` | Public JWKS on the loopback listener |
| `AssumeRoleWithSAML` / `AssumeRoleWithWebIdentity` | Federation token crypto (not SigV4) |
| Lambda Function URL with AuthType `NONE` | Open invoke on `/lambda-url/...` |
| HTTP API routes with authorizer `NONE` | Open invoke on `/http-api/...` |
| AppSync GraphQL | `API_KEY`, `AWS_IAM` (SigV4), or Cognito User Pools Bearer JWT |

All other AWS API paths require a valid SigV4 signature (header or query) for a known access key.

Additional auth notes:

- Temporary credentials require a matching `X-Amz-Security-Token`.
- Presigned S3 GET/PUT use query SigV4 (`X-Amz-Expires` max 604800).
- `GetCallerIdentity` succeeds after SigV4 without an IAM permission check.
- Lab S3, SQS, Lambda, ECR, SNS, Secrets Manager, and DynamoDB dataplane paths use same-account identity **or** resource policy Allow, and cross-account identity **and** resource policy Allow.
- Lab KMS APIs use `EvaluateKMS` (key policy explicit allow or grant, always required, plus identity for cross-account).
- Lab Lambda configure APIs use identity Evaluate plus PassRole/trust.
- Organizations SCP/RCP filters apply on member authorize paths, including OU-path inheritance.
- SNS HTTP(S) subscription endpoints must be loopback or an explicit lab catcher. Arbitrary operator URLs are rejected.
- Cognito management APIs require SigV4 and identity Allow.
- API Gateway JWT routes reject missing or invalid Bearer tokens. IAM routes reject unsigned requests. HTTP API resource policies are not invented.
- Gateway CredentialsArn and CodeDeploy serviceRoleArn require PassRole plus matching service trust when set.
- Deferred depth returns `501 NotImplemented` or an explicit fail-closed error after successful authn. Never silent Allow.

## Optional TLS

Set both `NOCTAXRIS_TLS_CERT` and `NOCTAXRIS_TLS_KEY` to serve TLS. If either is empty, the server uses plain HTTP.
