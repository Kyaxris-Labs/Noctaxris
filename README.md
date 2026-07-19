# Noctaxris

Local AWS emulator for cloud security labs. IAM-faithful and secure by default.

Repo and Go module: `github.com/Kyaxris-Labs/Noctaxris` (PascalCase `Noctaxris`, not all-lowercase).

Primary distribution: Docker image under [Kyaxris-Labs](https://github.com/Kyaxris-Labs).

## Quick start (Phase 3)

1. Copy `docker/.env.example` to `docker/.env` and set root access keys.
2. `docker compose -f docker/compose.yaml --env-file docker/.env up --build`
3. `curl http://127.0.0.1:4566/_noctaxris/health`
4. Point AWS CLI at the endpoint with the same root keys (see [docs/verification.md](docs/verification.md)).

```bash
aws sts get-caller-identity --endpoint-url http://127.0.0.1:4566
aws iam create-user --user-name labuser --endpoint-url http://127.0.0.1:4566
aws organizations create-account --email member@example.com --account-name Member --endpoint-url http://127.0.0.1:4566
```

Phase 3 covers SigV4, lab-complete IAM, all 11 STS actions with fail-closed federation, and Organizations CreateAccount / AssumeRole. See [docs/phases/phase-3.md](docs/phases/phase-3.md) and [docs/deferred.md](docs/deferred.md).

## Defaults

- Host publish `127.0.0.1:4566` only
- No `docker.sock`
- Root credentials via env injection
- Secrets encrypted at rest under the data volume
- SigV4 required on non-health paths (except SAML/OIDC federation STS)

## Docs

See [docs/index.md](docs/index.md) for architecture, configuration, security, verification, and phase history.
