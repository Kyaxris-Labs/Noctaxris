# CodeBuild lab build image

**Status:** shipped (image + docs)

Optional Alpine-based tool image for nested CodeBuild: `curl`, `jq`, pip `boto3`, and pip `awscli`. Non-root UID 1000. Source: `docker/codebuild-lab/`.

## Image allowlist

Noctaxris `AllowImagePull` already permits lab registry refs shaped as `ACCOUNT/REPO:tag` (12-digit account, single path segment). Use:

| Context | Image URI |
|---------|-----------|
| CreateProject / CLI | `127.0.0.1:4566/ACCOUNT/codebuild-lab:1.0.0` |
| DinD pull (automatic rewrite) | `host.docker.internal:<listenPort>/ACCOUNT/codebuild-lab:1.0.0` |

Do not set `environment.image` to arbitrary Docker Hub tags. Unallowlisted hosts fail closed. Extra prefixes via `NOCTAXRIS_IMAGE_PULL_ALLOWLIST` still require digests for registry hosts.

Pinned public bases such as `alpine:3.20` remain available for minimal smoke; this image is for labs that need AWS-shaped tooling inside the build container.

## Build and push

```bash
docker build -f docker/codebuild-lab/Dockerfile -t noctaxris-codebuild-lab:1.0.0 docker/codebuild-lab

ACCOUNT=$(aws sts get-caller-identity --endpoint-url "$EP" --query Account --output text)
aws ecr create-repository --repository-name codebuild-lab --endpoint-url "$EP" || true
PASS=$(aws ecr get-login-password --endpoint-url "$EP")
echo "$PASS" | docker login --username AWS --password-stdin 127.0.0.1:4566
docker tag noctaxris-codebuild-lab:1.0.0 "127.0.0.1:4566/${ACCOUNT}/codebuild-lab:1.0.0"
docker push "127.0.0.1:4566/${ACCOUNT}/codebuild-lab:1.0.0"
```

Compose must include `noctaxris-engine` so Registry V2 manifest PUT syncs the image into DinD. ECR smoke detail: [ecr.md](ecr.md#how-to-verify--cli-smoke).

## Project wiring

Point CodeBuild `environment.image` at the lab registry URI above. Service role needs pull permissions on that repository. Nested StartBuild runs `/bin/sh -c` with minted `AWS_*` and `AWS_ENDPOINT_URL_*` (see [codebuild.md](codebuild.md)).

## Not yet / deferred

- Official AWS CodeBuild curated images / Amazon Linux aws-cli v2 packaging in this tree (pip awscli is intentional for Alpine size)
- Publishing this image to Docker Hub or other public registries as a pull path
