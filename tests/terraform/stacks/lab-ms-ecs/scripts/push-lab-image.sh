#!/usr/bin/env bash
# Push alpine:3.20 into a lab ECR repository for live ECS pulls.
# Usage: push-lab-image.sh <endpoint> <repository-name> [tag]
set -euo pipefail

EP="${1:?endpoint required}"
REPO="${2:?repository name required}"
TAG="${3:-lab}"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not on PATH" >&2
  exit 1
fi
if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI not on PATH" >&2
  exit 1
fi

ACCOUNT="$(aws sts get-caller-identity --endpoint-url "$EP" --query Account --output text)"
PASS="$(aws ecr get-login-password --endpoint-url "$EP")"
echo "$PASS" | docker login --username AWS --password-stdin 127.0.0.1:4566
docker pull alpine:3.20
IMAGE="127.0.0.1:4566/${ACCOUNT}/${REPO}:${TAG}"
docker tag alpine:3.20 "$IMAGE"
docker push "$IMAGE"
echo "$IMAGE"
