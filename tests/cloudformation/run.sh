#!/usr/bin/env bash
# CloudFormation create/describe/delete via AWS CLI against Noctaxris.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TEMPLATES="$(cd "$(dirname "$0")/templates" && pwd)"

export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-AKIAROOTEXAMPLE01}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_EC2_METADATA_DISABLED="${AWS_EC2_METADATA_DISABLED:-true}"
EP="${NOCTAXRIS_ENDPOINT:-http://127.0.0.1:4566}"

if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI not on PATH — skip CloudFormation CLI suite" >&2
  exit 0
fi

if ! curl -fsS "$EP/_noctaxris/ready" | grep -q ready; then
  echo "Noctaxris not ready at $EP — skip CloudFormation CLI suite" >&2
  exit 0
fi

SUFFIX="$(date +%s)$RANDOM"
BUCKET="cfn-cli-${SUFFIX}"
BUCKET="$(echo "$BUCKET" | tr '[:upper:]' '[:lower:]' | cut -c1-63)"
ROLE="CfnCliRole${SUFFIX}"
STACK="cfn-cli-${SUFFIX}"

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/noctaxris-cfn.XXXXXX")"
cleanup() {
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

sed -e "s/REPLACE_BUCKET_NAME/${BUCKET}/g" -e "s/REPLACE_ROLE_NAME/${ROLE}/g" \
  "$TEMPLATES/s3-and-iam.json" >"$WORKDIR/stack.json"

aws cloudformation create-stack \
  --stack-name "$STACK" \
  --template-body "file://$WORKDIR/stack.json" \
  --endpoint-url "$EP"

aws cloudformation describe-stacks --stack-name "$STACK" --endpoint-url "$EP" \
  --query 'Stacks[0].StackStatus' --output text | grep -E 'CREATE_COMPLETE|CREATE_IN_PROGRESS'

aws cloudformation list-stacks --endpoint-url "$EP" \
  --query "StackSummaries[?StackName=='${STACK}'].StackName" --output text | grep -qx "$STACK"

aws s3api head-bucket --bucket "$BUCKET" --endpoint-url "$EP"

aws cloudformation delete-stack --stack-name "$STACK" --endpoint-url "$EP"

echo "CloudFormation CLI suite succeeded (stack=$STACK)"
