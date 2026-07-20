# Lambda

**Status:** shipped

Lab-complete Lambda with zip and container image packaging, versions and aliases, layers, sync and async Invoke, function resource policies (including lab cross-account principals), multi-runtime zip support, nested DinD compute with TLS to the engine, execution-role session injection, and platform egress deny for function containers.

## Implemented

| Area | Behavior |
|------|----------|
| APIs | `CreateFunction`, `GetFunction`, `DeleteFunction`, `ListFunctions`, `UpdateFunctionCode`, `UpdateFunctionConfiguration`, `Invoke`, `PublishVersion`, `ListVersionsByFunction`, `CreateAlias`, `UpdateAlias`, `DeleteAlias`, `GetAlias`, `ListAliases`, `PublishLayerVersion`, `GetLayerVersion`, `ListLayerVersions`, `DeleteLayerVersion`, `AddPermission`, `RemovePermission`, `GetPolicy`, `CreateEventSourceMapping`, `GetEventSourceMapping`, `ListEventSourceMappings`, `UpdateEventSourceMapping`, `DeleteEventSourceMapping`, `CreateFunctionUrlConfig`, `GetFunctionUrlConfig`, `DeleteFunctionUrlConfig`, `ListFunctionUrlConfigs` |
| Packaging | Zip upload (`Code.ZipFile`) or `PackageType=Image` with `Code.ImageUri` (lab ECR refs pull with Registry V2 auth) |
| Runtimes (zip) | `python3.11`, `python3.12`, `nodejs20.x` |
| Versions | `PublishVersion` freezes code and config. `$LATEST` stays mutable |
| Aliases | Point at published version numbers. Invoke accepts bare name, `name:version`, or `name:alias` |
| Layers | Up to five same-account layer-version ARNs per function. Merged at `/opt` on zip and Image Invoke |
| Invoke (sync) | `InvocationType=RequestResponse` (default). One-shot nested container |
| Invoke (async) | `InvocationType=Event` returns HTTP 202 immediately. Two lab retries, then SQS DLQ via `DeadLetterConfig.TargetArn` or SQS/SNS via `DestinationConfig.OnFailure` |
| SQS ESM | `CreateEventSourceMapping` for SQS ARNs only. In-process poller ReceiveMessage → sync Invoke → DeleteMessage on success. Lab `BatchSize` max 10. Disable stops polling |
| Function URLs | `CreateFunctionUrlConfig` with `AuthType` `NONE` or `AWS_IAM`. Lab invoke path `http://127.0.0.1:4566/lambda-url/ACCOUNT/FUNCTION`. AuthType `NONE` returns simple CORS headers (`Access-Control-Allow-Origin: *`) including OPTIONS preflight |
| Role configure | Caller needs `iam:PassRole` on the role ARN. Role trust must Allow `sts:AssumeRole` for `lambda.amazonaws.com` |
| Resource policy | `AddPermission`, `RemovePermission`, `GetPolicy`. Same-account Invoke allows identity **or** function policy Allow. Cross-account Invoke requires identity **and** function policy Allow |
| Compute | Nested containers via Compose `noctaxris-engine` (DinD, TLS on port 2376). Default runtime. No host `docker.sock` on the API container. Opt-in microVM (`NOCTAXRIS_COMPUTE_RUNTIME=microvm`) on Linux with KVM and a Firecracker binary. WSL2 is DinD-only. Missing KVM or binary fails closed without host Docker |
| Invoke session | Temporary AWS_* credentials for the function execution role injected into the container |
| Egress | Function network `noctaxris-fn` with `Internal: true` (platform egress deny) |

Zip contents live under `$DATAROOT/lambda/...` and are shared with DinD through the Compose data volume. Compose sets `NOCTAXRIS_DOCKER_HOST=tcp://noctaxris-engine:2376` and `NOCTAXRIS_DOCKER_CERT_PATH=/certs/client`. The engine API stays on the Compose network only. Empty `NOCTAXRIS_DOCKER_HOST` disables DinD compute so unit tests can run without DinD. Without the engine, sync Invoke on the default DinD path returns compute unavailable.

### Compute runtime matrix

| Host | DinD (default) | Opt-in microVM |
|------|----------------|----------------|
| Linux with usable `/dev/kvm` and Firecracker binary | Supported | Supported when opted in (guest invoke may still be deferred until assets ship) |
| WSL2 | Supported (default and only nested path) | Unsupported (fails closed) |
| Windows native (no Linux VM) | Unsupported for nested compute | Unsupported |

Set `NOCTAXRIS_COMPUTE_RUNTIME=dind` (or leave unset) for the default path. Set `microvm` only on Linux/KVM labs. Optional `NOCTAXRIS_FIRECRACKER_BIN` points at the Firecracker binary when it is not on `PATH`. Opt-in never falls through to host Docker.

Image functions pull `ImageUri` inside DinD. Lab one-shot Invoke supports AWS Lambda Python base images and compatible `python:` or `nodejs:` refs. For private lab images, push to the ECR lab registry ([ecr.md](ecr.md)) and reference `127.0.0.1:4566/ACCOUNT/REPO:tag` in `Code.ImageUri`. Invoke issues a lab ECR authorization token and pulls with Registry V2 auth (same path as ECS RunTask). Public images are unchanged.

### Authz notes

Lambda configure APIs use identity `EvaluateFull` plus `CheckPassRole` (caller `iam:PassRole` and Lambda service trust). Data-plane APIs including Invoke use `authorizeDataplaneOR` with the function owner account from the function ARN. Same-account access: allow if identity **or** function policy Allows. Cross-account access (Invoke with a full function ARN in another lab account): allow only when identity **and** function policy both Allow. Empty function policy denies cross-account callers. Explicit Deny wins. Boundary and SCP/RCP apply when identity Allows.

`AddPermission` accepts other lab account IAM user/role ARNs, account root ARNs, or a 12-digit lab account id (stored as account root). Wildcard `*` principals remain rejected. Service principal cross-account grants stay deferred.

`CreateAlias`, `UpdateAlias`, `DeleteAlias`, and `ListAliases` share short names with KMS. Send `X-Amz-Target: AWSLambda.CreateAlias` (and similar) so JSON requests route to Lambda. Query-string `Action=CreateAlias` still maps to KMS.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine` so sync Invoke can start a nested container. Prefer WSL or Linux against `http://127.0.0.1:4566`.

Zip create and sync Invoke:

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

PublishVersion, alias, and Invoke by qualifier:

```bash
aws lambda publish-version --function-name "$FN" --endpoint-url "$EP"
aws lambda create-alias \
  --function-name "$FN" \
  --name live \
  --function-version 1 \
  --endpoint-url "$EP"

aws lambda invoke \
  --function-name "${FN}:live" \
  --payload '{}' \
  --cli-binary-format raw-in-base64-out \
  /tmp/noctaxris-alias-out.json \
  --endpoint-url "$EP"
```

Function resource policy (guest invoke when policy Allows):

```bash
aws lambda add-permission \
  --function-name "$FN" \
  --statement-id guest \
  --action lambda:InvokeFunction \
  --principal arn:aws:iam::000000000001:root \
  --endpoint-url "$EP"

aws lambda get-policy --function-name "$FN" --endpoint-url "$EP"
```

### Advanced smoke

Async Invoke (`InvocationType=Event`). Expect HTTP 202 and `StatusCode` 202. Background work still needs DinD for a successful run. Without the engine the API still returns 202 then the lab worker fails after retries.

```bash
aws lambda invoke \
  --function-name "$FN" \
  --invocation-type Event \
  --payload '{"async":true}' \
  --cli-binary-format raw-in-base64-out \
  /tmp/noctaxris-async-out.json \
  --endpoint-url "$EP"

cat /tmp/noctaxris-async-out.json
```

Image CreateFunction (`PackageType=Image`). Lab one-shot Invoke supports AWS Lambda Python base images such as `public.ecr.aws/lambda/python:3.12`. Pull happens inside DinD.

```bash
IMG_FN="noctaxris-img-$RANDOM"
aws lambda create-function \
  --function-name "$IMG_FN" \
  --package-type Image \
  --role "$ROLE_ARN" \
  --code ImageUri=public.ecr.aws/lambda/python:3.12 \
  --endpoint-url "$EP"

aws lambda get-function --function-name "$IMG_FN" --endpoint-url "$EP"
```

Layer publish and attach (max five same-account layer-version ARNs). Zip and Image Invoke merge layer contents at `/opt`.

```bash
mkdir -p /tmp/noctaxris-layer/python
echo 'LAYER=1' > /tmp/noctaxris-layer/python/marker.txt
(cd /tmp/noctaxris-layer && zip -qr /tmp/noctaxris-layer.zip .)

LAYER_NAME="noctaxris-layer-$RANDOM"
aws lambda publish-layer-version \
  --layer-name "$LAYER_NAME" \
  --zip-file fileb:///tmp/noctaxris-layer.zip \
  --compatible-runtimes python3.12 \
  --endpoint-url "$EP"

LAYER_ARN=$(aws lambda list-layer-versions \
  --layer-name "$LAYER_NAME" \
  --endpoint-url "$EP" \
  --query 'LayerVersions[0].LayerVersionArn' \
  --output text)

aws lambda update-function-configuration \
  --function-name "$FN" \
  --layers "$LAYER_ARN" \
  --endpoint-url "$EP"

aws lambda get-function --function-name "$FN" --endpoint-url "$EP"
```

SQS event source mapping (in-process poller). Lab `BatchSize` max is 10. Sync Invoke deletes messages on success. Without DinD the mapping still creates, but poller Invoke fails and messages stay until visibility timeout.

```bash
aws sqs create-queue --queue-name "noctaxris-esm-$RANDOM" --endpoint-url "$EP"
Q_ARN=$(aws sqs get-queue-attributes --queue-url ... --attribute-names QueueArn --endpoint-url "$EP" --query Attributes.QueueArn --output text)
aws lambda create-event-source-mapping \
  --function-name "$FN" \
  --event-source-arn "$Q_ARN" \
  --batch-size 5 \
  --endpoint-url "$EP"
```

Function URL lite. AuthType `NONE` skips SigV4 on `http://127.0.0.1:4566/lambda-url/ACCOUNT/FUNCTION` and sets CORS allow-origin `*` (OPTIONS returns 204). AuthType `AWS_IAM` requires SigV4 plus `lambda:InvokeFunctionUrl`. For JWT-protected HTTP fronts, use API Gateway HTTP API rather than Function URLs.

```bash
aws lambda create-function-url-config \
  --function-name "$FN" \
  --auth-type NONE \
  --endpoint-url "$EP"
```

Expect CreateFunction to succeed only when the role trusts `lambda.amazonaws.com`. Expect sync Invoke to return JSON with `"ok": true` when DinD is up. Expect alias Invoke to hit the published version. Expect async Invoke to print `StatusCode` 202. Without `NOCTAXRIS_DOCKER_HOST`, sync Invoke returns compute unavailable.

Two-account cross-account Invoke (member account B owns the function, member account A user invokes via dual eval):

```bash
# Account B: create function then grant account A principal
aws lambda add-permission \
  --function-name "$FN" \
  --statement-id xa-invoke \
  --action lambda:InvokeFunction \
  --principal arn:aws:iam::ACCOUNT_A:user/invoker \
  --endpoint-url "$EP" --profile account-b

# Account A: identity policy plus function policy must both Allow
FN_ARN="arn:aws:lambda:us-east-1:ACCOUNT_B:function:$FN"
aws lambda invoke \
  --function-name "$FN_ARN" \
  --payload '{"xa":true}' \
  --cli-binary-format raw-in-base64-out \
  /tmp/noctaxris-xa-out.json \
  --endpoint-url "$EP" --profile account-a
```

## Not yet / deferred

- Full Lambda SAR (provisioned concurrency, weighted alias routing, SnapStart, VPC ENI, recursive loop protection depth, tags, tracing, code signing)
- EventBridge or Lambda-to-Lambda failure destinations (OnFailure to SQS and SNS is shipped)
- Service-principal cross-account grants on function policies
- Non-lab private registries (Docker Hub private, third-party hosts). Lab ECR on `127.0.0.1:4566` is supported for Image Invoke
- FilterCriteria / ReportBatchItemFailures / provisioned pollers / non-SQS ESM sources (Kinesis, DynamoDB Streams, MQ)
- Function URL CORS configuration object depth (simple ACAO headers on NONE are shipped). Prefer API Gateway HTTP API for JWT labs. CloudFront is a config stub only (see [cloudfront.md](cloudfront.md))
- Rootless DinD
- Live Firecracker guest zip/Image Invoke on Linux+KVM (opt-in selection and fail-closed probe ship. Real guest boot awaits a Linux+KVM host with kernel/rootfs assets)

### Opt-in microVM

MicroVM isolation selection is opt-in via `NOCTAXRIS_COMPUTE_RUNTIME=microvm`. DinD remains the default. Live Firecracker guest invoke for zip and Image packaging is still deferred until a Linux+KVM lab host completes asset packaging. ECS RunTask uses the same opt-in selection and fail-closed stubs. See [ecs.md](ecs.md) and [index.md](index.md#cross-cutting).
