#!/usr/bin/env bash
# Nested DinD operator/CI smoke for Noctaxris.
# Requires Docker Compose, curl, and AWS CLI v2.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE=(docker compose -f "$ROOT/docker/compose.yaml" --env-file "$ROOT/docker/.env")
EP="${EP:-http://127.0.0.1:4566}"
READY_TIMEOUT_SEC="${READY_TIMEOUT_SEC:-180}"
KEEP_UP="${KEEP_UP:-0}"

cleanup() {
  if [[ "$KEEP_UP" == "1" ]]; then
    return 0
  fi
  "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -f "$ROOT/docker/.env" ]]; then
  cp "$ROOT/docker/.env.example" "$ROOT/docker/.env"
fi

# shellcheck disable=SC1091
set -a
source "$ROOT/docker/.env"
set +a

export AWS_ACCESS_KEY_ID="${NOCTAXRIS_ROOT_ACCESS_KEY_ID:?missing NOCTAXRIS_ROOT_ACCESS_KEY_ID}"
export AWS_SECRET_ACCESS_KEY="${NOCTAXRIS_ROOT_SECRET_ACCESS_KEY:?missing NOCTAXRIS_ROOT_SECRET_ACCESS_KEY}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_EC2_METADATA_DISABLED=true

echo "==> compose up (API + nested engine)"
"${COMPOSE[@]}" up --build -d

echo "==> wait for GET $EP/_noctaxris/ready (engine TLS dial included when Docker host set)"
deadline=$((SECONDS + READY_TIMEOUT_SEC))
until curl -fsS "$EP/_noctaxris/ready" 2>/dev/null | grep -q ready; do
  if (( SECONDS >= deadline )); then
    echo "timeout waiting for ready" >&2
    "${COMPOSE[@]}" ps >&2 || true
    "${COMPOSE[@]}" logs --no-color --tail=80 >&2 || true
    exit 1
  fi
  sleep 2
done

# Ready implies engine TLS dial; also require Compose health=healthy on noctaxris-engine.
ENGINE_CID="$("${COMPOSE[@]}" ps -q noctaxris-engine)"
ENGINE_HEALTH="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$ENGINE_CID" 2>/dev/null || true)"
if [[ "$ENGINE_HEALTH" != "healthy" ]]; then
  echo "noctaxris-engine health=$ENGINE_HEALTH (want healthy)" >&2
  "${COMPOSE[@]}" ps >&2 || true
  exit 1
fi
echo "==> noctaxris-engine healthy"

echo "==> STS get-caller-identity"
aws sts get-caller-identity --endpoint-url "$EP" >/dev/null

echo "==> nested RDS create + describe"
ID="smoke-pg-$RANDOM"
aws rds create-db-instance \
  --db-instance-identifier "$ID" \
  --db-instance-class db.t3.micro \
  --engine postgres \
  --master-username postgres \
  --master-user-password 'lab-smoke-password-1' \
  --allocated-storage 20 \
  --db-name postgres \
  --endpoint-url "$EP" >/dev/null

DESC="$(aws rds describe-db-instances --db-instance-identifier "$ID" --endpoint-url "$EP" --output json)"
echo "$DESC" | grep -E '"Address"|Endpoint|DBInstanceStatus|Status' || true
if echo "$DESC" | grep -Eq '127\.0\.0\.1|0\.0\.0\.0'; then
  echo "nested endpoint must not publish host loopback addresses" >&2
  exit 1
fi
if ! echo "$DESC" | grep -Eq 'noctaxris-data-rds-|DBInstanceStatus'; then
  echo "describe missing nested RDS endpoint shape" >&2
  echo "$DESC" >&2
  exit 1
fi

DB_ARN="$(aws rds describe-db-instances --db-instance-identifier "$ID" --endpoint-url "$EP" --query 'DBInstances[0].DBInstanceArn' --output text)"
SECRET_ARN="$(aws rds describe-db-instances --db-instance-identifier "$ID" --endpoint-url "$EP" --query 'DBInstances[0].MasterUserSecret.SecretArn' --output text)"

# Nested start can lag briefly after create.
STATUS=""
for _ in $(seq 1 45); do
  STATUS="$(aws rds describe-db-instances --db-instance-identifier "$ID" --endpoint-url "$EP" --query 'DBInstances[0].DBInstanceStatus' --output text)"
  if [[ "$STATUS" == "available" ]]; then
    break
  fi
  sleep 2
done
if [[ "$STATUS" != "available" ]]; then
  echo "RDS instance never became available (status=$STATUS)" >&2
  exit 1
fi

echo "==> RDS Data API ExecuteStatement (require nested-psql when DinD started Postgres)"
EXEC_OUT="$(aws rds-data execute-statement \
  --resource-arn "$DB_ARN" \
  --secret-arn "$SECRET_ARN" \
  --database postgres \
  --sql 'SELECT 1 AS n' \
  --endpoint-url "$EP" \
  --output json)"
echo "$EXEC_OUT" | grep -E 'records|formattedRecords|noctaxrisExecutor' || true
if ! echo "$EXEC_OUT" | grep -q 'records'; then
  echo "ExecuteStatement missing records" >&2
  echo "$EXEC_OUT" >&2
  exit 1
fi
if ! echo "$EXEC_OUT" | grep -Fq 'nested-psql'; then
  echo "ExecuteStatement must use nested-psql executor (stub is not enough for DinD smoke)" >&2
  echo "$EXEC_OUT" >&2
  exit 1
fi

echo "==> optional Lambda zip Invoke (skip with SKIP_LAMBDA=1)"
if [[ "${SKIP_LAMBDA:-0}" != "1" ]]; then
  TRUST='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}'
  aws iam create-role --role-name SmokeLambdaRole --assume-role-policy-document "$TRUST" --endpoint-url "$EP" >/dev/null || true
  ROLE_ARN="$(aws iam get-role --role-name SmokeLambdaRole --endpoint-url "$EP" --query Role.Arn --output text)"
  TMP="$(mktemp -d)"
  cat >"$TMP/handler.py" <<'PY'
def handler(event, context):
    return {"ok": True}
PY
  (cd "$TMP" && zip -q "$TMP/fn.zip" handler.py)
  FN="smoke-fn-$RANDOM"
  aws lambda create-function \
    --function-name "$FN" \
    --runtime python3.12 \
    --role "$ROLE_ARN" \
    --handler handler.handler \
    --zip-file "fileb://$TMP/fn.zip" \
    --endpoint-url "$EP" >/dev/null
  OUT="$TMP/out.json"
  if aws lambda invoke --function-name "$FN" --payload '{}' --endpoint-url "$EP" "$OUT" >/dev/null; then
    grep -q '"ok"' "$OUT" || { echo "invoke payload unexpected: $(cat "$OUT")" >&2; exit 1; }
    echo "Lambda Invoke ok"
  else
    echo "Lambda Invoke failed (engine/image pull); see compose logs" >&2
    exit 1
  fi
fi

echo "smoke-nested OK"
