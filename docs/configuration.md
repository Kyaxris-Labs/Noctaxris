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

## Data layout under `NOCTAXRIS_DATA_ROOT`

| Path | Role |
|------|------|
| `master.key` | 32-byte AEAD key (mode `0600` when created) |
| `state.db` | SQLite accounts and access keys |
| `cloudtrail/events.jsonl` | Audit trail |

## Docker / Compose

Files live under `docker/`:

- `Dockerfile`: multi-stage build (`golang:1.26-bookworm` → distroless nonroot), `CGO_ENABLED=0`
- `compose.yaml`: publish `127.0.0.1:4566:4566`, named volume for data, `read_only: true`, tmpfs `/tmp`, no `docker.sock`
- `.env.example`: sample root keys for local Compose

Copy `.env.example` to `.env`, set real lab keys, then:

```bash
docker compose -f docker/compose.yaml --env-file docker/.env up --build
curl http://127.0.0.1:4566/_noctaxris/health
```

Root `.dockerignore` keeps `.env`, git metadata, and similar junk out of the build context.
