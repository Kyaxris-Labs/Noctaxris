#!/usr/bin/env bash
# Terraform apply + destroy against a running Noctaxris API.
# STACK=lab-core (default) or STACK=lab-fullstack
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
STACK_NAME="${STACK:-lab-core}"
STACK="$(cd "$(dirname "$0")/stacks/${STACK_NAME}" && pwd)"

export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-AKIAROOTEXAMPLE01}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_EC2_METADATA_DISABLED="${AWS_EC2_METADATA_DISABLED:-true}"
EP="${NOCTAXRIS_ENDPOINT:-http://127.0.0.1:4566}"

if ! command -v terraform >/dev/null 2>&1; then
  echo "terraform not on PATH — skip Terraform suite" >&2
  exit 0
fi

if ! curl -fsS "$EP/_noctaxris/ready" | grep -q ready; then
  echo "Noctaxris not ready at $EP — skip Terraform suite" >&2
  exit 0
fi

PREFIX="tf$(date +%s)$(printf '%04d' $RANDOM)"
# S3 bucket names must be lowercase DNS-style.
PREFIX="$(echo "$PREFIX" | tr '[:upper:]' '[:lower:]')"

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/noctaxris-tf.XXXXXX")"
cleanup() {
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

cp -a "$STACK/." "$WORKDIR/"
cd "$WORKDIR"

# Shared plugin cache avoids full registry downloads; retries cover flaky TLS to registry.terraform.io.
export TF_PLUGIN_CACHE_DIR="${TF_PLUGIN_CACHE_DIR:-${HOME}/.terraform.d/plugin-cache}"
mkdir -p "$TF_PLUGIN_CACHE_DIR"
init_ok=0
for attempt in 1 2 3 4 5 6 7 8; do
  if terraform init -input=false -no-color; then
    init_ok=1
    break
  fi
  sleep 2
done
if [[ "$init_ok" -ne 1 ]]; then
  echo "terraform init failed after retries (registry unreachable?)" >&2
  exit 1
fi

terraform apply -input=false -auto-approve -no-color \
  -var="endpoint=$EP" \
  -var="region=$AWS_DEFAULT_REGION" \
  -var="access_key=$AWS_ACCESS_KEY_ID" \
  -var="secret_key=$AWS_SECRET_ACCESS_KEY" \
  -var="name_prefix=$PREFIX"

# Assert key outputs are non-empty (stack-specific names differ).
OUTPUT_JSON="$(terraform output -json -no-color)"
STACK_NAME="$STACK_NAME" OUTPUT_JSON="$OUTPUT_JSON" python3 - <<'PY'
import json, os, sys
outs = json.loads(os.environ["OUTPUT_JSON"])
stack = os.environ["STACK_NAME"]
required = {
    "lab-core": ["bucket_name", "role_arn", "table_name", "kms_key_id"],
    "lab-fullstack": [
        "bucket_name", "kms_key_arn", "table_name", "lambda_role_arn",
        "queue_url", "topic_arn", "function_name", "event_bus_name",
        "rule_name", "ssm_parameter_name", "ssm_stringlist_parameter_name", "secret_arn",
    ],
}.get(stack, [])
missing = [k for k in required if not outs.get(k, {}).get("value")]
if missing:
    sys.exit(f"empty terraform outputs for {stack}: {missing}")
print(f"terraform outputs ok for {stack} ({len(required)} keys)")
PY

terraform destroy -input=false -auto-approve -no-color \
  -var="endpoint=$EP" \
  -var="region=$AWS_DEFAULT_REGION" \
  -var="access_key=$AWS_ACCESS_KEY_ID" \
  -var="secret_key=$AWS_SECRET_ACCESS_KEY" \
  -var="name_prefix=$PREFIX"

echo "Terraform apply+destroy succeeded (stack=$STACK_NAME prefix=$PREFIX)"
