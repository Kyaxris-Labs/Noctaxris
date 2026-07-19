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
- Secrets are sealed with ChaCha20-Poly1305 before landing in SQLite.
- Tests assert the plaintext secret does not appear in `state.db`.
- Audit events must not carry secret material.

## Auth (Phase 1)

- Health is open for container checks: `GET /_noctaxris/health`.
- Every other path requires a valid SigV4 signature (header or query) for a known access key.
- Unknown keys, bad signatures, and skewed clocks return `403` with an AWS-shaped XML error and an audit line.
- `GetCallerIdentity` succeeds after SigV4 without an IAM permission check.
- Other actions return `501 NotImplemented` after successful authn.

## Optional TLS

Set both `NOCTAXRIS_TLS_CERT` and `NOCTAXRIS_TLS_KEY` to serve TLS. If either is empty, the server uses plain HTTP.
