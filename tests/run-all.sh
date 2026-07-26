#!/usr/bin/env bash
# Run SDK, Terraform, and CloudFormation integration suites against Noctaxris.
#
# Optional env (default suite stays fast):
#   NOCTAXRIS_ADVANCED=1  — SDK fullstack + Terraform lab-fullstack + Lambda runtime + MS stacks
#   TF_LAMBDA_RUNTIMES=1  — Terraform lab-lambda-{python,nodejs,java} only (after lab-core)
#   TF_MS=1               — Terraform lab-ms-serverless + lab-ms-ecs (live=false)
#   TF_MS_LIVE=1          — Also lab-ms-ecs with live=true (needs DinD; soft-skip if docker down)
#   NOCTAXRIS_NESTED=1    — SDK Lambda Invoke (needs healthy noctaxris-engine)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-AKIAROOTEXAMPLE01}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_EC2_METADATA_DISABLED="${AWS_EC2_METADATA_DISABLED:-true}"
export NOCTAXRIS_ENDPOINT="${NOCTAXRIS_ENDPOINT:-http://127.0.0.1:4566}"

EP="$NOCTAXRIS_ENDPOINT"
if ! curl -fsS "$EP/_noctaxris/ready" | grep -q ready; then
  echo "Noctaxris not ready at $EP — start Compose first (see tests/README.md)" >&2
  exit 1
fi

SDK_TIMEOUT="10m"
if [[ "${NOCTAXRIS_ADVANCED:-}" == "1" || "${NOCTAXRIS_NESTED:-}" == "1" ]]; then
  SDK_TIMEOUT="20m"
fi

echo "==> SDK (Go) [ADVANCED=${NOCTAXRIS_ADVANCED:-0} NESTED=${NOCTAXRIS_NESTED:-0}]"
(cd tests/sdk/go && go test ./... -count=1 -timeout "$SDK_TIMEOUT")

echo "==> SDK (Node.js)"
(cd tests/sdk/nodejs && npm install --no-fund --no-audit && npm test)

echo "==> SDK (Python)"
(cd tests/sdk/python && python -m pip install -q -r requirements.txt && python -m pytest)

echo "==> Terraform (lab-core)"
bash tests/terraform/run.sh

if [[ "${TF_FULLSTACK:-}" == "1" || "${NOCTAXRIS_ADVANCED:-}" == "1" ]]; then
  echo "==> Terraform (lab-fullstack) [TF_FULLSTACK=${TF_FULLSTACK:-0} ADVANCED=${NOCTAXRIS_ADVANCED:-0}]"
  STACK=lab-fullstack bash tests/terraform/run.sh
fi

if [[ "${TF_LAMBDA_RUNTIMES:-}" == "1" || "${NOCTAXRIS_ADVANCED:-}" == "1" ]]; then
  echo "==> Terraform (Lambda runtime stacks) [TF_LAMBDA_RUNTIMES=${TF_LAMBDA_RUNTIMES:-0} ADVANCED=${NOCTAXRIS_ADVANCED:-0}]"
  for stack in lab-lambda-python lab-lambda-nodejs lab-lambda-java; do
    STACK="$stack" bash tests/terraform/run.sh
  done
fi

if [[ "${TF_MS:-}" == "1" || "${NOCTAXRIS_ADVANCED:-}" == "1" ]]; then
  echo "==> Terraform (microservice stacks) [TF_MS=${TF_MS:-0} ADVANCED=${NOCTAXRIS_ADVANCED:-0}]"
  STACK=lab-ms-serverless bash tests/terraform/run.sh
  STACK=lab-ms-ecs bash tests/terraform/run.sh
fi

if [[ "${TF_MS_LIVE:-}" == "1" ]]; then
  echo "==> Terraform (lab-ms-ecs live) [TF_MS_LIVE=1]"
  STACK=lab-ms-ecs TF_MS_LIVE=1 bash tests/terraform/run.sh
fi

echo "==> CloudFormation"
(cd tests/cloudformation/go && go test ./... -count=1 -timeout 5m)

echo "All integration suites passed."
