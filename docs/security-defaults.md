# Security defaults

These defaults are intentional product posture for a local emulator that people will point SDKs and AWS CLI at.

## Host and container

- Compose publishes only `127.0.0.1:4566` on the host. That is not `0.0.0.0` on the host.
- Inside the container the process listens on `0.0.0.0:4566` so the published mapping works.
- No `docker.sock` mount. Nested compute (later phases) must not regain host Docker control via a socket.
- Image runs as distroless `nonroot`. Data dir is seeded owned by UID `65532` so the volume is writable.
- Compose sets `read_only: true` with `/tmp` as tmpfs.

## Credentials and crypto

- Injected env root credentials become the initial account root.
- Access-key secrets, KMS CMK material, and SSE-S3 DEKs are sealed with ChaCha20-Poly1305 before landing in SQLite.
- Object ciphertext for SSE lives under the data volume filesystem.
- Tests assert plaintext secrets and CMK bytes do not appear in `state.db`.
- Audit events must not carry secret or plaintext key material.
- Inactive access keys are rejected at SigV4 verification.

## Auth (Phase 5)

- Health is open for container checks: `GET /_noctaxris/health`.
- Every other path requires a valid SigV4 signature (header or query) for a known access key.
- Temporary credentials require a matching `X-Amz-Security-Token`.
- Presigned S3 GET/PUT use query SigV4 (`X-Amz-Expires` max 604800).
- `GetCallerIdentity` succeeds after SigV4 without an IAM permission check.
- Lab S3 APIs use `EvaluateS3` (identity or bucket policy union).
- Lab KMS APIs use `EvaluateKMS` (key policy explicit allow or grant).
- Deferred depth returns `501 NotImplemented` or an explicit fail-closed error after successful authn. Never silent Allow.

## Optional TLS

Set both `NOCTAXRIS_TLS_CERT` and `NOCTAXRIS_TLS_KEY` to serve TLS. If either is empty, the server uses plain HTTP.
