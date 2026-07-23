#!/usr/bin/env bash
# Run SDK, Terraform, and CloudFormation integration suites against Noctaxris.
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

echo "==> SDK (Go)"
(cd tests/sdk/go && go test ./... -count=1 -timeout 10m)

echo "==> SDK (Node.js)"
(cd tests/sdk/nodejs && npm install --no-fund --no-audit && npm test)

echo "==> SDK (Python)"
(cd tests/sdk/python && python -m pip install -q -r requirements.txt && python -m pytest)

echo "==> Terraform"
bash tests/terraform/run.sh

echo "==> CloudFormation"
(cd tests/cloudformation/go && go test ./... -count=1 -timeout 5m)

echo "All integration suites passed."
