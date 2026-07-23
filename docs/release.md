# Release checklist

How to cut a public Noctaxris release (example: **1.0.0**). Docker Hub image: **`kyaxris/noctaxris`**.

## Secrets (GitHub Actions)

Set on the canonical repo [`Kyaxris-Labs/Noctaxris`](https://github.com/Kyaxris-Labs/Noctaxris) (Settings → Secrets and variables → Actions). Never commit credentials.

| Secret | Purpose |
|--------|---------|
| `DOCKERHUB_USERNAME` | Docker Hub account or org username that owns `kyaxris/noctaxris` |
| `DOCKERHUB_TOKEN` | Docker Hub [access token](https://docs.docker.com/docker-hub/access-tokens/) with push rights (not the account password) |

Forks skip publish with a log line. Missing secrets fail closed on schedule and on dispatch for the canonical repo.

## Before the tag

1. Bump `VERSION` (plain text, e.g. `1.0.0`) and keep `internal/version/version.go` default in sync.
2. Move CHANGELOG notes under `## 1.0.0` (feature-oriented sections; no internal delivery labels).
3. Confirm PR CI is green (`unit`, `image`, `smoke-core`). Run nested smoke when the release touches DinD / Compose engine paths (`docker/smoke-nested.sh` or Actions `nested_smoke=true`).
4. Confirm docs still describe DinD-only compute and loopback defaults.

## Cut the release

```bash
# On the commit you intend to ship (main tip after merge):
git tag -a v1.0.0 -m "Noctaxris 1.0.0"
git push origin v1.0.0
```

Pushing tag `v*` runs [`.github/workflows/release.yml`](../.github/workflows/release.yml), which builds `docker/Dockerfile` and pushes:

| Tag | Meaning |
|-----|---------|
| `kyaxris/noctaxris:1.0.0` | Exact semver |
| `kyaxris/noctaxris:1.0` | Major.minor |
| `kyaxris/noctaxris:1` | Major |
| `kyaxris/noctaxris:latest` | Latest tagged release |
| `kyaxris/noctaxris:sha-<short>` | Git short SHA |

Then create the GitHub Release for `v1.0.0` (UI or `gh release create v1.0.0 --notes-file ...`) using the CHANGELOG `1.0.0` section.

Optional: Actions → **release** → Run workflow with an existing tag if you need to re-push Hub tags after fixing secrets.

## Nightly (separate)

[`.github/workflows/docker-nightly.yml`](../.github/workflows/docker-nightly.yml) runs on a UTC cron and `workflow_dispatch`. It pushes `nightly`, `nightly-YYYYMMDD`, and `sha-<short>` only. It does **not** move `latest` or semver tags.

## Local image check

```bash
docker build -f docker/Dockerfile --build-arg VERSION=1.0.0 -t kyaxris/noctaxris:local .
docker run --rm kyaxris/noctaxris:local # needs env; or extract binary version via compose
curl -sS http://127.0.0.1:4566/_noctaxris/version
# 1.0.0
```

## Related

- Ops / CI matrix: [ops.md](ops.md)
- Security posture: [security-defaults.md](security-defaults.md)
