# Lambda

**Status:** shipped

Lab-complete Lambda with zip and container image packaging, versions and aliases, layers, sync and async Invoke, same-account resource policies, multi-runtime zip support, nested DinD compute with TLS to the engine, execution-role session injection, and platform egress deny for function containers.

## Implemented

| Area | Behavior |
|------|----------|
| APIs | `CreateFunction`, `GetFunction`, `DeleteFunction`, `ListFunctions`, `UpdateFunctionCode`, `UpdateFunctionConfiguration`, `Invoke`, `PublishVersion`, `ListVersionsByFunction`, `CreateAlias`, `UpdateAlias`, `DeleteAlias`, `GetAlias`, `ListAliases`, `PublishLayerVersion`, `GetLayerVersion`, `ListLayerVersions`, `DeleteLayerVersion`, `AddPermission`, `RemovePermission`, `GetPolicy` |
| Packaging | Zip upload (`Code.ZipFile`) or `PackageType=Image` with `Code.ImageUri` |
| Runtimes (zip) | `python3.11`, `python3.12`, `nodejs20.x` |
| Versions | `PublishVersion` freezes code and config. `$LATEST` stays mutable |
| Aliases | Point at published version numbers. Invoke accepts bare name, `name:version`, or `name:alias` |
| Layers | Up to five same-account layer-version ARNs per function. Merged at `/opt` on zip Invoke |
| Invoke (sync) | `InvocationType=RequestResponse` (default). One-shot nested container |
| Invoke (async) | `InvocationType=Event` returns HTTP 202 immediately. Two lab retries, then SQS DLQ via `DeadLetterConfig.TargetArn` or `DestinationConfig.OnFailure` |
| Role configure | Caller needs `iam:PassRole` on the role ARN. Role trust must Allow `sts:AssumeRole` for `lambda.amazonaws.com` |
| Resource policy | Same-account `AddPermission`, `RemovePermission`, `GetPolicy`. Invoke allows identity **or** function policy Allow |
| Compute | Nested containers via Compose `noctaxris-engine` (DinD, TLS on port 2376). No host `docker.sock` on the API container |
| Invoke session | Temporary AWS_* credentials for the function execution role injected into the container |
| Egress | Function network `noctaxris-fn` with `Internal: true` (platform egress deny) |

Zip contents live under `$DATAROOT/lambda/...` and are shared with DinD through the Compose data volume. Compose sets `NOCTAXRIS_DOCKER_HOST=tcp://noctaxris-engine:2376` and `NOCTAXRIS_DOCKER_CERT_PATH=/certs/client`. The engine API stays on the Compose network only. Empty `NOCTAXRIS_DOCKER_HOST` disables compute so unit tests can run without DinD. Without the engine, sync Invoke returns compute unavailable.

Image functions pull `ImageUri` inside DinD. Lab one-shot Invoke supports AWS Lambda Python base images and compatible `python:` or `nodejs:` refs. For private lab images, push to the ECR lab registry ([ecr.md](ecr.md)) and reference `127.0.0.1:4566/ACCOUNT/REPO:tag` in `Code.ImageUri`.

### Authz notes

Lambda configure APIs use identity `EvaluateFull` plus `CheckPassRole` (caller `iam:PassRole` and Lambda service trust). Data-plane APIs including Invoke use identity **or** function resource policy Allow via `authorizeDataplaneOR` (explicit Deny wins, boundary and SCP/RCP when identity Allows). Invoke still mints temporary credentials for the function role so in-function SDK calls can hit the same emulator endpoint (`NOCTAXRIS_LAMBDA_ENDPOINT_URL`).

`CreateAlias`, `UpdateAlias`, `DeleteAlias`, and `ListAliases` share short names with KMS. Send `X-Amz-Target: AWSLambda.CreateAlias` (and similar) so JSON requests route to Lambda. Query-string `Action=CreateAlias` still maps to KMS.

Cross-account principals and service principals on function policies are rejected in the lab.

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

Layer publish and attach (max five same-account layer-version ARNs). Zip Invoke merges layer contents at `/opt`. Image Invoke does not mount layers.

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

Expect CreateFunction to succeed only when the role trusts `lambda.amazonaws.com`. Expect sync Invoke to return JSON with `"ok": true` when DinD is up. Expect alias Invoke to hit the published version. Expect async Invoke to print `StatusCode` 202. Without `NOCTAXRIS_DOCKER_HOST`, sync Invoke returns compute unavailable.

## Not yet / deferred

- Full Lambda SAR (event source mappings, provisioned concurrency, weighted alias routing, Function URLs, SnapStart, VPC ENI, recursive loop protection depth, tags, tracing, code signing)
- EventBridge or Lambda-to-Lambda failure destinations (SQS DLQ and OnFailure to SQS only)
- Cross-account function resource policy depth
- Non-lab private registries (use ECR lab registry for account-local images)
- Layers mounted on Image Invoke (layers can be attached in the API but are not mounted during image Invoke)
- Rootless DinD and microVM isolation (Firecracker-class, post-v2)

### Post-v2

MicroVM isolation (Firecracker-class) for Lambda. v2 keeps nested DinD. Later ECS may share the same isolation track if needed. See [index.md](index.md#cross-cutting).
