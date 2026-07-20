# Security defaults

These defaults are intentional product posture for a local emulator that people will point SDKs and AWS CLI at.

## Host and container

- Compose publishes only `127.0.0.1:4566` on the host. That is not `0.0.0.0` on the host.
- Inside the container the process listens on `0.0.0.0:4566` so the published mapping works.
- No host `docker.sock` mount on the API service. Nested compute uses Compose service `noctaxris-engine` over TLS on the Compose network (`NOCTAXRIS_DOCKER_HOST=tcp://noctaxris-engine:2376`, `NOCTAXRIS_DOCKER_CERT_PATH=/certs/client`). The engine API is not published to the host.
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

## Auth (Phase 7)

- Health is open for container checks: `GET /_noctaxris/health`.
- Every other path requires a valid SigV4 signature (header or query) for a known access key.
- Temporary credentials require a matching `X-Amz-Security-Token`.
- Presigned S3 GET/PUT use query SigV4 (`X-Amz-Expires` max 604800).
- `GetCallerIdentity` succeeds after SigV4 without an IAM permission check.
- Lab S3, SQS, Lambda, ECR, SNS, Secrets Manager, and DynamoDB dataplane paths use same-account identity **or** resource policy Allow, and cross-account identity **and** resource policy Allow.
- Lab KMS APIs use `EvaluateKMS` (key policy explicit allow or grant, always required, plus identity for cross-account).
- Lab Lambda configure APIs use identity Evaluate plus PassRole/trust.
- Organizations SCP/RCP filters apply on member authorize paths, including OU-path inheritance.
- Deferred depth returns `501 NotImplemented` or an explicit fail-closed error after successful authn. Never silent Allow.

## Optional TLS

Set both `NOCTAXRIS_TLS_CERT` and `NOCTAXRIS_TLS_KEY` to serve TLS. If either is empty, the server uses plain HTTP.
