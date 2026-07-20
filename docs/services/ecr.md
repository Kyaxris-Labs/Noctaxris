# ECR

**Status:** shipped

Lab-complete ECR core: repository CRUD, authorization tokens, repository policies, image metadata APIs, and a Docker Registry HTTP API V2 path on the same loopback listener (`127.0.0.1:4566`). JSON control plane via `X-Amz-Target: AmazonEC2ContainerRegistry_V*` or service `ecr` with JSON body.

## Implemented

| Area | Actions |
|------|---------|
| Repositories | `CreateRepository`, `DescribeRepositories`, `DeleteRepository` |
| Auth | `GetAuthorizationToken` (base64 `AWS:password`, lab proxy endpoint on loopback) |
| Repository policy | `GetRepositoryPolicy`, `SetRepositoryPolicy`, `DeleteRepositoryPolicy` |
| Images | `PutImage`, `BatchGetImage`, `ListImages`, `BatchDeleteImage` |
| Registry V2 | `GET /v2/`, blob upload (monolithic PUT), manifest GET/PUT/HEAD, tags list. `GET`/`POST` `/v2/token` returns Docker Registry token JSON (`token` / `access_token`) for the `GetAuthorizationToken` password. `WWW-Authenticate` Bearer realm points at `/v2/token` |
| DinD sync | On manifest PUT with a tag, pull the image into `noctaxris-engine` so ECS and Lambda Image paths can use lab registry refs |

Repository URI for docker login and push: `127.0.0.1:4566/ACCOUNT/REPOSITORY` (account from `sts get-caller-identity`). Blobs and manifests persist under the data volume.

### Authz notes

Repository-scoped APIs use `authorizeDataplaneOR` with the repository owner account from the repository ARN (or `registryId` when resolving). Same-account access: allow if identity **or** repository policy Allows. Cross-account access: allow only when identity **and** repository policy both Allow. Empty repository policy denies cross-account callers. Explicit Deny in either wins. Org SCP/RCP filters apply before evaluation. When identity Allows, permissions boundary and session intersect.

`GetAuthorizationToken` uses identity eval on `Resource: *`.

`CreateRepository` uses identity `EvaluateFull` on the repository ARN (no policy yet).

Pass `registryId` with `DescribeRepositories` or `BatchGetImage` to address another lab account registry.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine` so manifest PUT can sync images into DinD.

```bash
ACCOUNT=$(aws sts get-caller-identity --endpoint-url "$EP" --query Account --output text)
REPO="noctaxris-lab-$RANDOM"

aws ecr create-repository --repository-name "$REPO" --endpoint-url "$EP"

PASS=$(aws ecr get-login-password --endpoint-url "$EP")
echo "$PASS" | docker login --username AWS --password-stdin 127.0.0.1:4566

docker pull alpine:3.20
docker tag alpine:3.20 "127.0.0.1:4566/${ACCOUNT}/${REPO}:lab"
docker push "127.0.0.1:4566/${ACCOUNT}/${REPO}:lab"

aws ecr list-images --repository-name "$REPO" --endpoint-url "$EP"
```

Repository policy via `SetRepositoryPolicy`:

```bash
POLICY='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ecr:BatchGetImage","Resource":"*","Principal":{"AWS":"*"}}]}'

aws ecr set-repository-policy \
  --repository-name "$REPO" \
  --policy-text "$POLICY" \
  --endpoint-url "$EP"
```

Expect `create-repository` to return a repository URI under `127.0.0.1:4566`. Expect `docker login` to succeed after `get-login-password` (Docker exchanges Basic credentials at `/v2/token` for a Bearer token matching that password). Expect `docker push` to succeed. Expect `list-images` to show the `lab` tag after push.

Two-account cross-account describe (member account B owns the repository, member account A user describes via dual eval):

```bash
aws ecr set-repository-policy --repository-name "$REPO" --endpoint-url "$EP" --profile account-b \
  --policy-text '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::ACCOUNT_A:user/reader"},"Action":"ecr:DescribeRepositories","Resource":"*"}]}'
aws ecr describe-repositories --registry-id ACCOUNT_B --repository-names "$REPO" \
  --endpoint-url "$EP" --profile account-a
```

## Not yet / deferred

- Image scanning, replication, lifecycle policies, public galleries
- OCI referrers and multi-arch index depth beyond single manifest
- Chunked blob PATCH uploads (monolithic PUT only today)
- Rootless DinD and microVM isolation (Firecracker-class, post-v2)
