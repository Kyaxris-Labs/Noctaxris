# Verification

How to confirm a local Noctaxris **lab core** build (phases 0-7).

## Unit and integration tests

```bash
go test ./... -count=1
```

Condition-key catalogs and ADR-0005 §7 evaluation are covered by:

```bash
go test ./internal/catalog/conditionkeys ./internal/kernel/authz -count=1
```

## Compose up

```bash
cp docker/.env.example docker/.env   # set root keys if needed
docker compose -f docker/compose.yaml --env-file docker/.env up --build -d
curl http://127.0.0.1:4566/_noctaxris/health
```

Expect body `ok`. Compose publishes `127.0.0.1:4566` only and must not mount host `docker.sock`. Lambda Invoke needs the nested `noctaxris-engine` service from the same compose file.

## AWS CLI smoke (WSL or Linux)

Export root keys from `docker/.env`, then:

```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_DEFAULT_REGION=us-east-1
EP=http://127.0.0.1:4566

aws sts get-caller-identity --endpoint-url "$EP"

aws iam create-user --user-name labuser --endpoint-url "$EP"
aws iam create-access-key --user-name labuser --endpoint-url "$EP"

TRUST='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}'
aws iam create-role --role-name LabRole --assume-role-policy-document "$TRUST" --endpoint-url "$EP"
aws sts assume-role --role-arn arn:aws:iam::000000000001:role/LabRole --role-session-name lab --endpoint-url "$EP"

aws organizations create-account --email member@example.com --account-name Member --endpoint-url "$EP"
aws sts get-session-token --endpoint-url "$EP"

# Fail-closed without IdP (unsigned federation call):
aws sts assume-role-with-web-identity \
  --role-arn arn:aws:iam::000000000001:role/LabRole \
  --role-session-name wid \
  --web-identity-token eyJhbGciOiJub25lIn0.e30. \
  --endpoint-url "$EP"
```

Expect `AccessDenied` / `IdP not configured` for the last command.

## IAM, Organizations, and STS smoke

Groups inherit attached and inline policies. Permissions boundaries intersect with identity policies in `EvaluateFull`.

```bash
aws iam create-group --group-name Admins --endpoint-url "$EP"
aws iam create-user --user-name alice --endpoint-url "$EP"
aws iam add-user-to-group --group-name Admins --user-name alice --endpoint-url "$EP"

LIST_DOC='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:ListUsers","Resource":"*"}]}'
aws iam create-policy --policy-name GroupListUsers --policy-document "$LIST_DOC" --endpoint-url "$EP"
POLICY_ARN=$(aws iam list-policies --scope Local --query "Policies[?PolicyName=='GroupListUsers'].Arn" --output text --endpoint-url "$EP")
aws iam attach-group-policy --group-name Admins --policy-arn "$POLICY_ARN" --endpoint-url "$EP"
```

Boundary deny: identity allows `ListUsers` but the boundary allows only `GetUser`.

```bash
aws iam create-user --user-name bounded --endpoint-url "$EP"
BOUNDED_KEYS=$(aws iam create-access-key --user-name bounded --endpoint-url "$EP" --output json)
BOUNDED_AKID=$(echo "$BOUNDED_KEYS" | python3 -c 'import sys,json; print(json.load(sys.stdin)["AccessKey"]["AccessKeyId"])')
BOUNDED_SECRET=$(echo "$BOUNDED_KEYS" | python3 -c 'import sys,json; print(json.load(sys.stdin)["AccessKey"]["SecretAccessKey"])')

BOUND_DOC='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:GetUser","Resource":"*"}]}'
aws iam create-policy --policy-name UserBoundary --policy-document "$BOUND_DOC" --endpoint-url "$EP"
BOUND_ARN=$(aws iam list-policies --scope Local --query "Policies[?PolicyName=='UserBoundary'].Arn" --output text --endpoint-url "$EP")
aws iam put-user-permissions-boundary --user-name bounded --permissions-boundary "$BOUND_ARN" --endpoint-url "$EP"
aws iam attach-user-policy --user-name bounded --policy-arn "$POLICY_ARN" --endpoint-url "$EP"

AWS_ACCESS_KEY_ID="$BOUNDED_AKID" AWS_SECRET_ACCESS_KEY="$BOUNDED_SECRET" \
  aws iam list-users --endpoint-url "$EP"
```

Expect `AccessDenied` for the last command.

Organizations list (management account `000000000001` plus any `CreateAccount` members):

```bash
aws organizations list-accounts --endpoint-url "$EP"
```

MFA and `GetSessionToken` (lab token, not RFC 6238 TOTP). After `CreateVirtualMFADevice`, `Base32StringSeed` is hex-encoded seed bytes. Token is the first 6 hex chars of `sha256(seed + ":" + unixMinute)` (see `internal/kernel/sts/mfa.go` and `sts_mfa_test.go`).

```bash
aws iam create-user --user-name mfa-user --endpoint-url "$EP"
MFA_KEYS=$(aws iam create-access-key --user-name mfa-user --endpoint-url "$EP" --output json)
MFA_AKID=$(echo "$MFA_KEYS" | python3 -c 'import sys,json; print(json.load(sys.stdin)["AccessKey"]["AccessKeyId"])')
MFA_SECRET=$(echo "$MFA_KEYS" | python3 -c 'import sys,json; print(json.load(sys.stdin)["AccessKey"]["SecretAccessKey"])')

MFA_JSON=$(aws iam create-virtual-mfa-device --endpoint-url "$EP" --output json)
SERIAL=$(echo "$MFA_JSON" | python3 -c 'import sys,json; print(json.load(sys.stdin)["VirtualMFADevice"]["SerialNumber"])')
SEED_HEX=$(echo "$MFA_JSON" | python3 -c 'import sys,json; print(json.load(sys.stdin)["VirtualMFADevice"]["Base32StringSeed"])')
aws iam enable-mfa-device --user-name mfa-user --serial-number "$SERIAL" --endpoint-url "$EP"

TOKEN=$(python3 -c "import hashlib,time; seed=bytes.fromhex('$SEED_HEX'); m=int(time.time())//60; print(hashlib.sha256(seed+b':'+str(m).encode()).hexdigest()[:6])")

AWS_ACCESS_KEY_ID="$MFA_AKID" AWS_SECRET_ACCESS_KEY="$MFA_SECRET" \
  aws sts get-session-token \
  --serial-number "$SERIAL" --token-code "$TOKEN" \
  --endpoint-url "$EP"
```

Expect a session with temporary credentials. `GetSessionToken` does not require an IAM `sts:GetSessionToken` permission after SigV4.

## KMS smoke (Phase 4)

```bash
KEY_JSON=$(aws kms create-key --endpoint-url "$EP" --output json)
KEY_ID=$(echo "$KEY_JSON" | python3 -c 'import sys,json; print(json.load(sys.stdin)["KeyMetadata"]["KeyId"])')

aws kms encrypt --key-id "$KEY_ID" --plaintext "$(echo -n hello | base64)" --endpoint-url "$EP"
aws kms generate-data-key --key-id "$KEY_ID" --key-spec AES_256 --endpoint-url "$EP"
aws kms create-alias --alias-name alias/lab --target-key-id "$KEY_ID" --endpoint-url "$EP"
```

## S3 smoke (Phase 5)

Force path-style addressing for the local endpoint:

```bash
aws configure set default.s3.addressing_style path

BUCKET="noctaxris-lab-$RANDOM"
aws s3 mb "s3://$BUCKET" --endpoint-url "$EP"
echo hello-s3 > /tmp/noctaxris-obj.txt
aws s3 cp /tmp/noctaxris-obj.txt "s3://$BUCKET/hello.txt" --endpoint-url "$EP"
aws s3 ls "s3://$BUCKET" --endpoint-url "$EP"
aws s3 cp "s3://$BUCKET/hello.txt" /tmp/noctaxris-obj-out.txt --endpoint-url "$EP"

# SSE-S3
aws s3 cp /tmp/noctaxris-obj.txt "s3://$BUCKET/sse-s3.txt" \
  --sse AES256 --endpoint-url "$EP"

# SSE-KMS (KEY_ID from Phase 4 create-key)
aws s3 cp /tmp/noctaxris-obj.txt "s3://$BUCKET/sse-kms.txt" \
  --sse aws:kms --sse-kms-key-id "$KEY_ID" --endpoint-url "$EP"

# Presign then curl (query SigV4, no Authorization header)
URL=$(aws s3 "presign" "s3://$BUCKET/hello.txt" --endpoint-url "$EP")
curl -fsS "$URL"
```

## DynamoDB smoke (Phase 6)

```bash
TABLE="noctaxris-lab-$RANDOM"
aws dynamodb create-table \
  --table-name "$TABLE" \
  --attribute-definitions AttributeName=pk,AttributeType=S \
  --key-schema AttributeName=pk,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url "$EP"

aws dynamodb put-item \
  --table-name "$TABLE" \
  --item '{"pk":{"S":"1"},"data":{"S":"hello-ddb"}}' \
  --endpoint-url "$EP"

aws dynamodb get-item \
  --table-name "$TABLE" \
  --key '{"pk":{"S":"1"}}' \
  --endpoint-url "$EP"
```

## SQS smoke (Phase 6)

```bash
QUEUE="noctaxris-lab-$RANDOM"
QUEUE_URL=$(aws sqs create-queue --queue-name "$QUEUE" --endpoint-url "$EP" --query QueueUrl --output text)

aws sqs send-message \
  --queue-url "$QUEUE_URL" \
  --message-body hello-sqs \
  --endpoint-url "$EP"

MSG_JSON=$(aws sqs receive-message --queue-url "$QUEUE_URL" --endpoint-url "$EP" --output json)
HANDLE=$(echo "$MSG_JSON" | python3 -c 'import sys,json; print(json.load(sys.stdin)["Messages"][0]["ReceiptHandle"])')

aws sqs delete-message \
  --queue-url "$QUEUE_URL" \
  --receipt-handle "$HANDLE" \
  --endpoint-url "$EP"
```

## Lambda smoke (Phase 7, WSL or Linux)

Compose must include `noctaxris-engine` so sync Invoke can start a nested container. Prefer WSL or Linux against `http://127.0.0.1:4566`.

```bash
LAMBDA_TRUST='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}'
aws iam create-role \
  --role-name LabLambdaRole \
  --assume-role-policy-document "$LAMBDA_TRUST" \
  --endpoint-url "$EP"

ROLE_ARN=$(aws iam get-role --role-name LabLambdaRole --endpoint-url "$EP" --query Role.Arn --output text)

mkdir -p /tmp/noctaxris-lambda
cat > /tmp/noctaxris-lambda/handler.py <<'PY'
def handler(event, context):
    return {"ok": True, "echo": event}
PY
(cd /tmp/noctaxris-lambda && zip -q /tmp/noctaxris-fn.zip handler.py)

FN="noctaxris-lab-$RANDOM"
aws lambda create-function \
  --function-name "$FN" \
  --runtime python3.12 \
  --role "$ROLE_ARN" \
  --handler handler.handler \
  --zip-file fileb:///tmp/noctaxris-fn.zip \
  --endpoint-url "$EP"

aws lambda invoke \
  --function-name "$FN" \
  --payload '{"ping":"pong"}' \
  --cli-binary-format raw-in-base64-out \
  /tmp/noctaxris-invoke-out.json \
  --endpoint-url "$EP"

cat /tmp/noctaxris-invoke-out.json
```

Expect CreateFunction to succeed only when the role trusts `lambda.amazonaws.com`. Expect Invoke to return JSON with `"ok": true` when DinD is up. Without `NOCTAXRIS_DOCKER_HOST`, Invoke returns compute unavailable.

On Windows, run the same commands inside WSL against `http://127.0.0.1:4566` when Docker Desktop publishes that port on the Windows host (WSL can reach it via `localhost` when mirrored networking is enabled, or use the Windows host IP).
