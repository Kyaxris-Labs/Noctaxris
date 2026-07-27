#!/usr/bin/env bash
# Terraform apply + destroy against a running Noctaxris API.
# STACK=lab-core (default), lab-fullstack, lab-lambda-*, lab-ms-serverless, lab-ms-ecs,
# lab-parity-observe, lab-parity-compute (also via TF_PARITY=1 / NOCTAXRIS_ADVANCED=1)
#
# lab-ms-ecs: set TF_MS_LIVE=1 for -var=live=true (DesiredCount>0; needs DinD).
#
# Lab-JSON edge/data surfaces (CloudFront, Transfer, Glue crawler, AppConfig, Config,
# SFN, CloudTrail, Firehose OS, Route53 Alias, ELBv2, AppSync, MQ, CloudWatch alarms)
# are SDK-only — do not add fake TF resources for them.
# When Compose publishes 127.0.0.1:4566 on a Windows host, skip this runner from WSL
# (WSL loopback is not the Windows host). Do not widen Compose publish to work around it.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
STACK_NAME="${STACK:-lab-core}"
STACK="$(cd "$(dirname "$0")/stacks/${STACK_NAME}" && pwd)"

export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-AKIAROOTEXAMPLE01}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_EC2_METADATA_DISABLED="${AWS_EC2_METADATA_DISABLED:-true}"
EP="${NOCTAXRIS_ENDPOINT:-http://127.0.0.1:4566}"

# WSL cannot reach Windows-host 127.0.0.1 publish. Skip when
# EP is loopback unless NOCTAXRIS_FORCE_WSL_TF=1. Do not widen Compose to 0.0.0.0.
is_wsl=0
if [[ -r /proc/version ]] && grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null; then
  is_wsl=1
fi
ep_host="${EP#*://}"
ep_host="${ep_host%%[:/]*}"
if [[ "$is_wsl" -eq 1 && -z "${NOCTAXRIS_FORCE_WSL_TF:-}" ]]; then
  if [[ "$ep_host" == "127.0.0.1" || "$ep_host" == "localhost" ]]; then
    echo "skip Terraform STACK=$STACK_NAME from WSL: endpoint $EP is loopback (Windows Compose publish is not this VM). Run from the host that shares the loopback, or set NOCTAXRIS_FORCE_WSL_TF=1 only when EP is reachable." >&2
    exit 0
  fi
fi

if ! command -v terraform >/dev/null 2>&1; then
  echo "terraform not on PATH — skip Terraform suite" >&2
  exit 0
fi

if ! curl -fsS "$EP/_noctaxris/ready" | grep -q ready; then
  echo "Noctaxris not ready at $EP — skip Terraform suite" >&2
  exit 0
fi

LIVE_VAR="false"
if [[ "$STACK_NAME" == "lab-ms-ecs" && "${TF_MS_LIVE:-}" == "1" ]]; then
  LIVE_VAR="true"
  # Soft-skip live DesiredCount>0 when nested engine looks unavailable.
  if ! docker info >/dev/null 2>&1; then
    echo "skip Terraform STACK=$STACK_NAME live=true: docker not usable (need host docker + healthy noctaxris-engine)" >&2
    exit 0
  fi
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

TF_VARS=(
  -var="endpoint=$EP"
  -var="region=$AWS_DEFAULT_REGION"
  -var="access_key=$AWS_ACCESS_KEY_ID"
  -var="secret_key=$AWS_SECRET_ACCESS_KEY"
  -var="name_prefix=$PREFIX"
)
if [[ "$STACK_NAME" == "lab-ms-ecs" ]]; then
  TF_VARS+=(-var="live=$LIVE_VAR")
fi

terraform apply -input=false -auto-approve -no-color "${TF_VARS[@]}"

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
    "lab-lambda-python": ["function_name", "runtime", "lambda_role_arn"],
    "lab-lambda-nodejs": ["function_name", "runtime", "lambda_role_arn"],
    "lab-lambda-java": [
        "function_name_java21", "function_name_java25",
        "runtime_java21", "runtime_java25", "lambda_role_arn",
    ],
    "lab-ms-serverless": [
        "bucket_name", "kms_key_arn", "table_name", "lambda_role_arn",
        "queue_url", "topic_arn", "function_name", "runtime",
        "event_bus_name", "rule_name", "ssm_parameter_name", "secret_arn",
    ],
    "lab-ms-ecs": [
        "repository_name", "task_definition_arn", "service_name",
        "desired_count", "task_role_arn", "execution_role_arn",
    ],
    "lab-parity-observe": ["table_name", "lsi_name"],
    "lab-parity-compute": [
        "launch_configuration_name", "asg_name", "asg_desired_capacity",
        "eks_cluster_name", "eks_cluster_arn", "eks_role_arn",
    ],
}.get(stack, [])
missing = [k for k in required if outs.get(k, {}).get("value") in (None, "")]
if missing:
    sys.exit(f"empty terraform outputs for {stack}: {missing}")
print(f"terraform outputs ok for {stack} ({len(required)} keys)")
PY

terraform destroy -input=false -auto-approve -no-color "${TF_VARS[@]}"

echo "Terraform apply+destroy succeeded (stack=$STACK_NAME prefix=$PREFIX live=${LIVE_VAR})"
