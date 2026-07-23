# Operations

Durable single-host lab ops for Noctaxris. This is not a multi-tenant HA guide.

## Single API replica

Run **one** Noctaxris API process against a given data root (Compose named volume or host path). Do not scale replicas against the same `state.db`. Multi-instance access is unsupported and can corrupt SQLite state. WAL is off by default; `busy_timeout=5000` applies on every connection. In-process workers (Scheduler, ESM pollers, Pipes ticker) assume a single API process.

Compose already mounts API state (`noctaxris-data`) separately from Lambda code shared with DinD (`noctaxris-compute`). `noctaxris-compute-init` chowns the compute volume to UID `65532` before the API starts. The nested engine mounts compute `:ro` and never mounts `master.key` or `state.db`. Default engine is restricted DinD (`privileged: false` with explicit caps). If nested smoke fails on your host: `docker compose -f docker/compose.yaml -f docker/compose.engine-privileged.yaml --env-file docker/.env up --build`.

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

## Published images

Docker Hub image: **`kyaxris/noctaxris`** (canonical GitHub repo `Kyaxris-Labs/Noctaxris` only).

| Tags | Source |
|------|--------|
| `1.x.y`, `1.x`, `1`, `latest`, `sha-<short>` | Tag push `v*` → [`.github/workflows/release.yml`](../.github/workflows/release.yml) |
| `nightly`, `nightly-YYYYMMDD`, `sha-<short>` | Nightly cron / dispatch → [`.github/workflows/docker-nightly.yml`](../.github/workflows/docker-nightly.yml) |

Repository secrets (never commit): `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`. Release cut steps: [release.md](release.md).

Product version: file `VERSION`, OCI label `org.opencontainers.image.version`, binary ldflags, and open probe `GET /_noctaxris/version`.

## Image upgrades

1. Stop Compose.
2. Take a backup (above).
3. Pull a Hub tag (`docker pull kyaxris/noctaxris:1.0.0`) or rebuild (`docker compose ... up --build`).
4. Start Compose and confirm `/_noctaxris/ready` returns `ready` (optional: `/_noctaxris/version`).

Schema changes are additive (`CREATE TABLE IF NOT EXISTS`, `ALTER TABLE ... ADD COLUMN` with duplicate-column ignore). A `schema_version` marker row exists (currently `1`) for operators and tests; migrations are still independent `Ensure*` helpers and do **not** consult that integer as a migration ledger. Do not treat the value as proof that a particular ALTER has applied. There is no down-migration. Prefer stop → backup → start over live multi-writer upgrades.

## Graceful shutdown

On `SIGTERM` or interrupt, the API stops in-process workers (Scheduler ticker, Lambda ESM poller, Pipes ticker, ECS service reconciler), then drains HTTP with a short shutdown timeout (about 10s). Prefer `docker compose ... stop` or `down` over `kill -9`. Backup still requires writers idle (Compose down) so SQLite and volume archives are consistent.

## Health vs ready

| Probe | Path | Meaning |
|-------|------|---------|
| Liveness | `GET /_noctaxris/health` | Process accepts HTTP |
| Readiness | `GET /_noctaxris/ready` | SQLite reachable; when `NOCTAXRIS_DOCKER_HOST` is set, engine TLS dial succeeds |

Compose `healthcheck` calls `/noctaxris healthcheck` (distroless, no curl) against readiness over the container's plain HTTP listener. Optional TLS (`NOCTAXRIS_TLS_CERT` / `NOCTAXRIS_TLS_KEY`) does not automatically switch Compose probes or client `http://` endpoint URLs; keep healthchecks and lab clients on HTTP unless you rewire both. The API waits on `noctaxris-compute-init` (`service_completed_successfully`) and `noctaxris-engine` (`service_healthy`).

## CI matrix

GitHub Actions (`.github/workflows/ci.yml`):

| Job | When |
|-----|------|
| unit / compose-static / govulncheck | Every push and PR (`go run ./scripts/govulncheck-ci`; allowlists only documented daemon-side Docker GO IDs with Fixed in: N/A) |
| race | Scoped `-race` on `internal/kernel` and `internal/store` |
| image | `docker build -f docker/Dockerfile .` |
| smoke-core | Every push and PR (after unit + compose-static + image): Compose up → ready → STS/S3/KMS/DynamoDB CLI; audit JSONL must not contain the root secret |
| smoke-nested | Weekly schedule on `main` plus Actions `workflow_dispatch` with `nested_smoke=true`. Runs `docker/smoke-nested.sh` (ready + engine healthy, nested RDS Describe, Data API nested-psql, Lambda zip/Image Invoke, short ECS RunTask). Skips cleanly if Docker is unavailable. **Not** on push/PR |
| integration-suites | Optional: `workflow_dispatch` with `integration_suites=true`, or pull requests that touch `tests/**`. Compose up → `tests/run-all.sh` (Go/Node/Python SDK, Terraform lab-core, CloudFormation). Dispatch inputs `advanced_suites` (`NOCTAXRIS_ADVANCED=1`) and `nested_sdk` (`NOCTAXRIS_NESTED=1`) opt into longer paths. **Not** a required PR gate |
| docker-nightly (separate workflow) | UTC cron + `workflow_dispatch`: build `docker/Dockerfile`, push `kyaxris/noctaxris:nightly` (+ dated / sha tags). Canonical repo + Hub secrets required |
| release (separate workflow) | Push tag `v*`: push semver + `latest` (+ sha). Same secrets. See [release.md](release.md) |

A green PR proves unit tests, image build, and `smoke-core` only. It does **not** prove nested DinD (Lambda Invoke, ECS, CodeBuild/Batch, nested RDS/ElastiCache/DocumentDB, Data API nested-psql). Run nested smoke via the weekly schedule, Actions `workflow_dispatch`, or the script below before relying on those paths. Any Compose change that touches `noctaxris-engine` privilege, devices, seccomp, or compute mounts must pass `docker/smoke-nested.sh` before merge; green `smoke-core` is not enough.

Operator shortcut (same script as the manual CI job):

```bash
cp docker/.env.example docker/.env   # if needed
bash docker/smoke-nested.sh
# Privileged engine opt-in (broken hosts only):
# COMPOSE_EXTRA_FILES="-f docker/compose.engine-privileged.yaml" bash docker/smoke-nested.sh
```

Per-service CLI smoke remains documented on each `docs/services/` page for operator runs outside CI.

## Compose overlays (lab opt-in)

Default `docker/compose.yaml` stays loopback-published and host-gateway off. Use overlays only for the labs that need them; do not make them the default stack.

| Overlay | When |
|---------|------|
| `docker/compose.engine-privileged.yaml` | Nested `docker info` / Invoke / nested data engines fail on restricted DinD (Desktop/WSL edge cases). Privileged DinD is a host workaround, not the secure default |
| `docker/compose.lab-host-gateway.yaml` | In-function SDK labs that call the published API via `host.docker.internal` (sets `NOCTAXRIS_INJECT_HOST_GATEWAY=1`). Keep code/default Compose off |
| `docker/compose.lab-open.yaml` | Open data-plane labs that need `AuthType NONE` / HTTP API `NONE` on the Compose non-loopback bind |

Desktop + nested DinD often cannot reach a loopback-only publish from function containers. For that layout only, with the host-gateway overlay, set session `NOCTAXRIS_PUBLISH_ADDR=0.0.0.0` (prefer TLS if the host is reachable beyond your lab machine). Restore `127.0.0.1` publish for all other work.

```bash
docker compose -f docker/compose.yaml -f docker/compose.lab-host-gateway.yaml --env-file docker/.env up --build
# Desktop DinD if connection refused from functions:
# NOCTAXRIS_PUBLISH_ADDR=0.0.0.0 docker compose -f docker/compose.yaml -f docker/compose.lab-host-gateway.yaml --env-file docker/.env up --build
```

## Nested OpenSearch host map count

OpenSearch nested create is **fail-closed** when the DinD host lacks adequate `vm.max_map_count` (`CreateFailed` + `stub://`). Noctaxris never raises host sysctl from the API container. When nested logs or wait errors match mmap / memory-lock bootstrap failures, CreateDomain / DescribeDomain include a lab `FailureReason` with the fix hint.

Raise the sysctl on the machine that runs DinD (Linux host or Docker Desktop/WSL VM) **before** CreateDomain if you need `Active`:

```bash
# Linux
cat /proc/sys/vm/max_map_count
sudo sysctl -w vm.max_map_count=262144

# Docker Desktop (Windows WSL2 backend) — inside the Desktop VM
wsl -d docker-desktop sysctl -w vm.max_map_count=262144
```

Persist on Linux via `/etc/sysctl.d/`; on Desktop/WSL see [services/opensearch.md](services/opensearch.md) (`.wslconfig` / Desktop VM steps). Control-plane-only labs that accept `CreateFailed` do not need the change.

## Related

- Env vars and data layout: [configuration.md](configuration.md)
- Security posture: [security-defaults.md](security-defaults.md)
