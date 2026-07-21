# Operations

Durable single-host lab ops for Noctaxris. This is not a multi-tenant HA guide.

## Single API replica

Run **one** Noctaxris API process against a given data root (Compose named volume or host path). Do not scale replicas against the same `state.db`. Multi-instance access is unsupported and can corrupt SQLite state. WAL is off by default; `busy_timeout=5000` applies on every connection. In-process workers (Scheduler, ESM pollers, Pipes ticker) assume a single API process.

Compose already mounts API state (`noctaxris-data`) separately from Lambda code shared with DinD (`noctaxris-compute`). `noctaxris-compute-init` chowns the compute volume to UID `65532` before the API starts. Privileged nested engine never mounts `master.key` or `state.db`.

## Backup and restore

1. Stop Compose so writers are idle:

```bash
docker compose -f docker/compose.yaml --env-file docker/.env down
```

2. Archive both volumes (or the host data root). Minimum set: `master.key`, `state.db`, `s3/`, `lambda/`, `cloudtrail/`. Also include `transfer/`, `transcribe/`, `bcm-exports/`, and `ecr/` when those labs were used.

```bash
docker run --rm -v noctaxris-data:/data -v "$PWD:/backup" busybox \
  tar czf /backup/noctaxris-data.tgz -C /data .
docker run --rm -v noctaxris-compute:/data -v "$PWD:/backup" busybox \
  tar czf /backup/noctaxris-compute.tgz -C /data .
```

Compose project prefixes may rename volumes (for example `docker_noctaxris-data`). Use `docker volume ls` and substitute the real names.

3. Restore into empty volumes or a fresh host path, confirm the files exist, then start Compose.

4. Without `master.key`, sealed secrets and CMK material cannot be decrypted even if `state.db` and object trees are restored.

## Image upgrades

1. Stop Compose.
2. Take a backup (above).
3. Pull or rebuild the API image (`docker compose ... up --build`).
4. Start Compose and confirm `/_noctaxris/ready` returns `ready`.

Schema changes are additive (`CREATE TABLE IF NOT EXISTS`, `ALTER TABLE ... ADD COLUMN` with duplicate-column ignore). A `schema_version` row records the applied level for operators and tests. There is no down-migration. Prefer stop → backup → start over live multi-writer upgrades.

## Health vs ready

| Probe | Path | Meaning |
|-------|------|---------|
| Liveness | `GET /_noctaxris/health` | Process accepts HTTP |
| Readiness | `GET /_noctaxris/ready` | SQLite reachable; when `NOCTAXRIS_DOCKER_HOST` is set, engine TLS dial succeeds |

Compose `healthcheck` calls `/noctaxris healthcheck` (distroless, no curl) against readiness. The API waits on `noctaxris-compute-init` (`service_completed_successfully`) and `noctaxris-engine` (`service_healthy`).

## CI matrix

GitHub Actions (`.github/workflows/ci.yml`):

| Job | When |
|-----|------|
| unit / compose-static / govulncheck | Every push and PR |
| race | Scoped `-race` on `internal/kernel` and `internal/store` |
| image | `docker build -f docker/Dockerfile .` |
| smoke-core | After unit + compose-static + image: Compose up → ready → STS/S3/KMS/DynamoDB CLI; audit JSONL must not contain the root secret |
| smoke-nested | Manual `workflow_dispatch` with `nested_smoke=true`: runs `docker/smoke-nested.sh` (ready + engine healthy, nested RDS Describe, Data API nested-psql, optional Lambda Invoke). Skips cleanly if Docker is unavailable |

Operator shortcut (same script as CI):

```bash
cp docker/.env.example docker/.env   # if needed
bash docker/smoke-nested.sh
```

Per-service CLI smoke remains documented on each `docs/services/` page for operator runs outside CI.

## Related

- Env vars and data layout: [configuration.md](configuration.md)
- Security posture: [security-defaults.md](security-defaults.md)
