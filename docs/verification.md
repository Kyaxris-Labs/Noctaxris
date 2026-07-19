# Verification

How to confirm a local Noctaxris build.

## Unit and integration tests

```bash
go test ./... -count=1
```

## Compose up

```bash
cp docker/.env.example docker/.env   # set root keys if needed
docker compose -f docker/compose.yaml --env-file docker/.env up --build -d
curl http://127.0.0.1:4566/_noctaxris/health
```

Expect body `ok`. Compose publishes `127.0.0.1:4566` only and must not mount `docker.sock`.

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
