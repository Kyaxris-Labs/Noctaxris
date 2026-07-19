# Noctaxris

Local AWS emulator for cloud security labs. IAM-faithful and secure by default.

Repo and Go module: `github.com/Kyaxris-Labs/Noctaxris` (PascalCase `Noctaxris`, not all-lowercase).

Primary distribution: Docker image under [Kyaxris-Labs](https://github.com/Kyaxris-Labs).

## Quick start (Phase 6)

1. Copy `docker/.env.example` to `docker/.env` and set root access keys.
2. `docker compose -f docker/compose.yaml --env-file docker/.env up --build`
3. `curl http://127.0.0.1:4566/_noctaxris/health`
4. Point AWS CLI at the endpoint with the same root keys (see [docs/verification.md](docs/verification.md)).

```bash
aws configure set default.s3.addressing_style path
aws sts get-caller-identity --endpoint-url http://127.0.0.1:4566
aws s3 mb s3://lab-bucket --endpoint-url http://127.0.0.1:4566
aws kms create-key --endpoint-url http://127.0.0.1:4566
```

Phase 6 adds lab-complete DynamoDB and SQS on top of S3, KMS, IAM, STS, and Organizations. See [docs/phases/phase-6.md](docs/phases/phase-6.md) and [docs/deferred.md](docs/deferred.md).

## Defaults

- Host publish `127.0.0.1:4566` only
- No `docker.sock`
- Root credentials via env injection
- Secrets and CMK material encrypted at rest under the data volume
- SigV4 required on non-health paths (except SAML/OIDC federation STS)

## Docs

See [docs/index.md](docs/index.md) for architecture, configuration, security, verification, and phase history.
