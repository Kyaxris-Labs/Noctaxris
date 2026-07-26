# CodeBuild lab image

Alpine image with `curl`, `jq`, pip `boto3`, and pip `awscli` for Noctaxris nested CodeBuild labs. Runs as UID 1000 (`codebuild`).

Push into the lab ECR registry. Nested builds pull that registry only (see [AllowImagePull](../../internal/compute/image_allow.go)): `127.0.0.1:4566/ACCOUNT/codebuild-lab:tag` or DinD `host.docker.internal:<port>/ACCOUNT/codebuild-lab:tag`. Random Docker Hub tags stay denied.

## Build

From the Noctaxris repo root:

```bash
docker build -f docker/codebuild-lab/Dockerfile -t noctaxris-codebuild-lab:1.0.0 docker/codebuild-lab
```

## Push to lab ECR

Noctaxris must be up with `noctaxris-engine` (DinD sync on manifest PUT). Shared CLI env: [docs/services/index.md](../../docs/services/index.md#shared-verification).

```bash
ACCOUNT=$(aws sts get-caller-identity --endpoint-url "$EP" --query Account --output text)
REPO=codebuild-lab
TAG=1.0.0

aws ecr create-repository --repository-name "$REPO" --endpoint-url "$EP" || true

PASS=$(aws ecr get-login-password --endpoint-url "$EP")
echo "$PASS" | docker login --username AWS --password-stdin 127.0.0.1:4566

docker tag noctaxris-codebuild-lab:1.0.0 "127.0.0.1:4566/${ACCOUNT}/${REPO}:${TAG}"
docker push "127.0.0.1:4566/${ACCOUNT}/${REPO}:${TAG}"
```

## Use in CreateProject

```bash
aws codebuild create-project \
  --name noctaxris-lab-tools \
  --service-role "$ROLE" \
  --source type=NO_SOURCE,buildspec='{"version":"0.2","phases":{"build":{"commands":["aws --version","python3 -c \"import boto3; print(boto3.__version__)\"","curl -fsS \"$AWS_ENDPOINT_URL\"/_noctaxris/health","jq -n .ok=true"]}}}' \
  --artifacts type=NO_ARTIFACTS \
  --environment type=LINUX_CONTAINER,image=127.0.0.1:4566/${ACCOUNT}/codebuild-lab:1.0.0,computeType=BUILD_GENERAL1_SMALL \
  --endpoint-url "$EP"
```

Service role must Allow `ecr:BatchGetImage` (and related pull actions) on the repository. Host-gateway for nested endpoint reachability: [codebuild.md](../../docs/services/codebuild.md#host-reachability).

Full notes: [codebuild-lab-image.md](../../docs/services/codebuild-lab-image.md).
