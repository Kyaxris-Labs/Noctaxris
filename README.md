# Noctaxris

Local AWS emulator for cloud security labs. IAM-faithful and secure by default.

Repo and Go module: `github.com/Kyaxris-Labs/Noctaxris` (PascalCase `Noctaxris`, not all-lowercase).

Primary distribution: Docker image under [Kyaxris-Labs](https://github.com/Kyaxris-Labs).

## Lab core (shipped)

Phases 0 through 7 are complete for lab use:

| Area | Lab surface |
|------|-------------|
| Identity | IAM users/roles/policies, full STS routing, Organizations CreateAccount |
| Crypto | KMS keys, key policies, Encrypt/Decrypt/GenerateDataKey, grants, aliases |
| Data | S3 (path-style, bucket policy, SSE-S3/SSE-KMS, presign), DynamoDB, SQS |
| Compute | Lambda zip `python3.12`, PassRole + trust, nested DinD Invoke |

Depth beyond this table lives in [docs/deferred.md](docs/deferred.md). Phase notes: [docs/phases/index.md](docs/phases/index.md).

## Quick start

1. Copy `docker/.env.example` to `docker/.env` and set root access keys.
2. `docker compose -f docker/compose.yaml --env-file docker/.env up --build`
3. `curl http://127.0.0.1:4566/_noctaxris/health` (expect `ok`)
4. Point AWS CLI at the endpoint with the same root keys.

```bash
export AWS_ACCESS_KEY_ID=...          # from docker/.env
export AWS_SECRET_ACCESS_KEY=...
export AWS_DEFAULT_REGION=us-east-1
EP=http://127.0.0.1:4566

aws configure set default.s3.addressing_style path
aws sts get-caller-identity --endpoint-url "$EP"
aws s3 mb s3://lab-bucket --endpoint-url "$EP"
aws kms create-key --endpoint-url "$EP"
```

Full smoke (IAM, Orgs, KMS, S3, DynamoDB, SQS, Lambda): [docs/verification.md](docs/verification.md).

## Defaults

- Host publish `127.0.0.1:4566` only
- No host `docker.sock` (nested `noctaxris-engine` for Lambda only)
- Root credentials via env injection
- Secrets and CMK material encrypted at rest under the data volume
- SigV4 required on non-health paths (except SAML/OIDC federation STS)
- Function containers: platform egress deny (unlike AWS Lambda default internet)

## Docs

See [docs/index.md](docs/index.md) for architecture, configuration, security, verification, and phase history.

## License

[MIT](LICENSE)
