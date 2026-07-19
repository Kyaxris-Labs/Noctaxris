# Lambda

**Status:** shipped

Lab-complete Lambda: zip packaging for `python3.12`, CreateFunction through UpdateConfiguration, sync Invoke, PassRole plus `lambda.amazonaws.com` trust, nested DinD compute (no host `docker.sock`), execution-role session injection, and platform egress deny for function containers.

## Implemented

| Area | Behavior |
|------|----------|
| APIs | `CreateFunction`, `GetFunction`, `DeleteFunction`, `ListFunctions`, `UpdateFunctionCode`, `UpdateFunctionConfiguration`, sync `Invoke` (`lambda:InvokeFunction`) |
| Packaging | Zip upload only (`Code.ZipFile`) |
| Runtime | `python3.12` only |
| Role configure | Caller needs `iam:PassRole` on the role ARN. Role trust must Allow `sts:AssumeRole` for `lambda.amazonaws.com` |
| Compute | Nested containers via Compose `noctaxris-engine` (DinD). No host `docker.sock` on the API container |
| Invoke session | Temporary AWS_* credentials for the function execution role injected into the container |
| Egress | Function network `noctaxris-fn` with `Internal: true` (platform egress deny) |

Zip contents live under `$DATAROOT/lambda/...` and are shared with DinD through the Compose data volume. Empty `NOCTAXRIS_DOCKER_HOST` disables compute so unit tests can run without DinD. Without the engine, Invoke returns compute unavailable.

### Authz notes

Lambda configure APIs use identity `EvaluateFull` plus `CheckPassRole` (caller `iam:PassRole` and Lambda service trust). Invoke requires identity Allow for `lambda:InvokeFunction` on the function ARN, then mints temporary credentials for the function role so in-function SDK calls can hit the same emulator endpoint (`NOCTAXRIS_LAMBDA_ENDPOINT_URL`).

Cross-account function resource policy depth is deferred. There is no lab Lambda resource-policy grant path yet.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine` so sync Invoke can start a nested container. Prefer WSL or Linux against `http://127.0.0.1:4566`.

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

## Not yet / deferred

- Full Lambda SAR beyond the lab set (layers, versions/aliases depth, event source mappings, concurrency, Function URLs, SnapStart, VPC ENI, recursive loop protection depth, tags, tracing, code signing, container image packaging, multi-runtime matrix)
- Async invoke, retries, DLQ / on-failure destinations
- Nested container engine hardening beyond lab DinD (rootless, TLS to engine by default)
- Cross-account function resource policy depth
- Runtimes other than `python3.12`

### Post-v2

MicroVM isolation (Firecracker-class) for Lambda. v2 keeps nested DinD. Later ECS may share the same isolation track if needed. See [index.md](index.md#cross-cutting).
