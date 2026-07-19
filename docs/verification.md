# Verification

How to confirm a local Noctaxris build.

## Unit and integration tests

```bash
go test ./... -count=1
```

## Compose up

```bash
cp docker/.env.example docker/.env   # set root keys if needed
docker compose -f docker/compose.yaml --env-file docker/.env up --build -d
curl http://127.0.0.1:4566/_noctaxris/health
```

Expect body `ok`. Compose publishes `127.0.0.1:4566` only and must not mount `docker.sock`.

## AWS CLI smoke (WSL or Linux)

Export root keys from `docker/.env`, then:

```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_DEFAULT_REGION=us-east-1
EP=http://127.0.0.1:4566

aws sts get-caller-identity --endpoint-url "$EP"

aws iam create-user --user-name labuser --endpoint-url "$EP"
aws iam create-access-key --user-name labuser --endpoint-url "$EP"
# use the returned user keys for:
aws sts get-caller-identity --endpoint-url "$EP"

TRUST='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}'
aws iam create-role --role-name LabRole --assume-role-policy-document "$TRUST" --endpoint-url "$EP"
aws sts assume-role --role-arn arn:aws:iam::000000000001:role/LabRole --role-session-name lab --endpoint-url "$EP"

aws organizations create-account --email member@example.com --account-name Member --endpoint-url "$EP"
aws sts get-session-token --endpoint-url "$EP"

# Fail-closed without IdP (unsigned federation call):
aws sts assume-role-with-web-identity \
  --role-arn arn:aws:iam::000000000001:role/LabRole \
  --role-session-name wid \
  --web-identity-token eyJhbGciOiJub25lIn0.e30. \
  --endpoint-url "$EP"
```

Expect `AccessDenied` / `IdP not configured` for the last command.

On Windows, run the same commands inside WSL against `http://127.0.0.1:4566` when Docker Desktop publishes that port on the Windows host (WSL can reach it via `localhost` when mirrored networking is enabled, or use the Windows host IP).
