# Lambda

**Status:** shipped

Lab-complete Lambda with zip and container image packaging, versions and aliases, layers, sync and async Invoke, function resource policies (including lab cross-account principals), multi-runtime zip support, nested DinD compute with TLS to the engine, execution-role session injection, and platform egress deny for function containers.

## Implemented

| Area | Behavior |
|------|----------|
| APIs | `CreateFunction`, `GetFunction`, `DeleteFunction`, `ListFunctions`, `UpdateFunctionCode`, `UpdateFunctionConfiguration`, `Invoke`, `PublishVersion`, `ListVersionsByFunction` (JSON target and REST `GET /{api}/functions/{name}/versions`; response includes `$LATEST`), `GetFunctionCodeSigningConfig` (lab: success without `CodeSigningConfigArn`), `PutFunctionEventInvokeConfig`, `GetFunctionEventInvokeConfig`, `DeleteFunctionEventInvokeConfig`, `CreateAlias`, `UpdateAlias`, `DeleteAlias`, `GetAlias`, `ListAliases`, `PublishLayerVersion`, `GetLayerVersion`, `ListLayerVersions`, `DeleteLayerVersion`, `AddPermission`, `RemovePermission`, `GetPolicy`, `CreateEventSourceMapping`, `GetEventSourceMapping`, `ListEventSourceMappings`, `UpdateEventSourceMapping`, `DeleteEventSourceMapping`, `CreateFunctionUrlConfig`, `GetFunctionUrlConfig`, `DeleteFunctionUrlConfig`, `ListFunctionUrlConfigs`, `ListTags` |
| Packaging | Zip upload (`Code.ZipFile`) or `PackageType=Image` with `Code.ImageUri` (lab ECR refs pull with Registry V2 auth) |
| Runtimes (zip) | `python3.11`, `python3.12`, `python3.13`, `python3.14`, `nodejs20.x`, `nodejs22.x`, `nodejs24.x`, `java21`, `java25` |
| Versions | `PublishVersion` freezes code and config. `$LATEST` stays mutable |
| Aliases | Point at published version numbers. Invoke accepts bare name, `name:version`, or `name:alias` |
| Layers | Up to five same-account layer-version ARNs per function. Merged at `/opt` on zip and Image Invoke. `FunctionConfiguration.Layers` returns AWS Layer objects (`Arn`, `CodeSize`), not bare ARN strings |
| Invoke (sync) | `InvocationType=RequestResponse` (default). One-shot nested container. Ships CloudWatch Logs to `/aws/lambda/{functionName}` with START / stdout (when available) / END / REPORT. Live audit includes function `resources[]` and `functionName` |
| Invoke (async) | `InvocationType=Event` returns HTTP 202 immediately. Two lab retries, then SQS DLQ via `DeadLetterConfig.TargetArn` or SQS/SNS via `DestinationConfig.OnFailure` (AWS object shape `{ Destination: arn }`; bare-string OnFailure accepted as lab alias). Event invoke config APIs set OnFailure and optional OnSuccess. S3 bucket notifications also enqueue async Invoke (`Records` event shape) when the function resource policy Allows `s3.amazonaws.com` with matching `aws:SourceArn` (not an Event Source Mapping) |
| SQS / DynamoDB Streams / Kinesis / MQ ESM | `CreateEventSourceMapping` for SQS queue ARNs, DynamoDB stream ARNs, Kinesis stream ARNs (`arn:aws:kinesis:region:account:stream/name`, up to four lab shards), or Amazon MQ broker ARNs (`arn:aws:mq:region:account:broker:NAME:ID`). FunctionName may include a version or alias qualifier (`name:1`, `name:live`); the mapping persists that qualifier and the poller Invokes it (not hard-coded `$LATEST`). Optional `FilterCriteria` (up to 5 Filters, OR of EventBridge-style JSON patterns): Create/Update validate patterns; poll applies filters before Invoke. SQS matches `body` (string or nested JSON object) and `messageId`; DynamoDB matches `eventName` plus `dynamodb.Keys` / `dynamodb.NewImage` / `dynamodb.OldImage`; Kinesis matches decoded `data` (nested JSON object when payload is JSON, else string) and `kinesis.partitionKey`. Supported operators: equals / list OR, `prefix`, `suffix`, `exists`, `anything-but`, `numeric`, `equals-ignore-case`. Unsupported (`$or`, `wildcard`, `cidr`, nested prefix/suffix equals-ignore-case) rejected at Create/Update. Optional `FunctionResponseTypes: ReportBatchItemFailures`: on invoke success, parse `batchItemFailures[].itemIdentifier` (SQS message id / DynamoDB or Kinesis sequence); delete only non-failed SQS messages; DynamoDB/Kinesis cursor advances only when the failure list is empty; malformed/unknown identifiers fail closed for the batch. Create/poll require the function `Role` session to Allow source actions (`sqs:ReceiveMessage`+`sqs:DeleteMessage`, `dynamodb:GetRecords`, `kinesis:GetRecords`+`kinesis:GetShardIterator`, or `mq:DescribeBroker` for MQ), or (SQS) a queue policy Allow for `lambda.amazonaws.com`. In-process poller: SQS ReceiveMessage → filter → sync Invoke → DeleteMessage on success (non-matching SQS messages are deleted without invoke); DynamoDB Streams / Kinesis GetRecords → filter → sync Invoke → advance cursor on success (non-matching records advance the cursor without invoke). Kinesis poll walks every stream shard sequentially with `GetRecords`, merges under `BatchSize`, and stores a JSON cursor map `shardId → sequence` (not Enhanced Fan-Out). Event shape is AWS-style (`Records[].kinesis.data` base64, `eventSource` `aws:kinesis`, `eventID` `shardId:sequence`); new mappings start at `TRIM_HORIZON` per shard. MQ Create requires the broker `RUNNING` with a nested non-`stub://` AMQP endpoint; otherwise `ValidationException`. MQ poll dials only allowlisted nested hosts (`noctaxris-mq-<broker-id>`; no host ports / no operator-supplied hosts). Lab destination queue name `noctaxris`. ActiveMQ events use `eventSource` `aws:mq` + `messages[]` (base64 `data`); RabbitMQ uses `aws:rmq` + `rmqMessagesByQueue`. Default receive (no injectable hook): RabbitMQ uses AMQP 0-9-1 `basic.get` (lab user `noctaxris` / `noctaxris-mq-lab`) and returns real bodies when the nested broker is `RUNNING`; ActiveMQ classic on 5672 speaks AMQP 1.0, so the path TCP-probes then returns an empty batch (no AMQP 1.0/JMS client). Dial/auth/protocol errors fail closed. Unit tests may still inject `MQReceiveFunc`; live MQ ESM smoke needs a nested broker (skip without engine). Lab `BatchSize` max 10. Disable stops polling. ESM poll Invokes also ship `/aws/lambda/*` logs and write a sibling CloudTrail `Invoke` line with `eventSourceMappingUUID`. |
| Function URLs | `CreateFunctionUrlConfig` with `AuthType` `NONE` or `AWS_IAM`. Lab invoke path `http://127.0.0.1:4566/lambda-url/ACCOUNT/FUNCTION`. AuthType `NONE` CORS: default `Access-Control-Allow-Origin: *`, or an allowlist via `Cors.AllowOrigins` / `NOCTAXRIS_FUNCTION_URL_CORS_ORIGINS` (comma-separated). On non-loopback listen (including default Compose), `NONE` requires `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1`. AuthType `AWS_IAM` requires SigV4 service `lambda` |
| Role configure | Caller needs `iam:PassRole` on the role ARN. Role trust must Allow `sts:AssumeRole` for `lambda.amazonaws.com` |
| Resource policy | `AddPermission`, `RemovePermission`, `GetPolicy`. Optional `FunctionUrlAuthType` / `InvokedViaFunctionUrl`; wildcard Principal only with `FunctionUrlAuthType`. Same-account Invoke allows identity **or** function policy Allow. Cross-account Invoke requires identity **and** function policy Allow. Lab service principals (`sns.amazonaws.com`, `events.amazonaws.com`, `s3.amazonaws.com`, `apigateway.amazonaws.com`, …) with optional foreign-lab `SourceAccount` / `SourceArn` Conditions support XA notify and RoleArn-less delivery |
| Compute | Nested containers via Compose `noctaxris-engine` (DinD, TLS on port 2376). No host `docker.sock` on the API container. Live zip/Image Invoke requires a healthy engine. Default engine is restricted (`privileged: false` + caps); `compose.engine-privileged.yaml` is opt-in for broken hosts |
| Invoke session | Temporary AWS_* credentials for the function execution role injected into the container |
| Egress | Function network `noctaxris-fn`: bridge with IP masquerade off (WAN deny). Host-gateway ExtraHosts is opt-in (`NOCTAXRIS_INJECT_HOST_GATEWAY=1` / `compose.lab-host-gateway.yaml`) for in-function SDK labs |
| VpcConfig | Rejected with ValidationException until ENI attachment exists (no silent drop) |
| Layers REST | CLI paths under `/2018-10-31/layers/...` (and `/2015-03-31/layers/...`) map to Publish/Get/List/Delete layer version |
| Policy / URL REST | `AddPermission` / `RemovePermission` / `GetPolicy` via `/2015-03-31/functions/{name}/policy`; Function URL config under `/2021-10-31/functions/{name}/url` |

Zip contents live under `$DATAROOT/lambda/...` and are shared with DinD through the Compose `noctaxris-compute` volume (API sealed state stays on `noctaxris-data` only; engine mounts compute `:ro`). Compose `noctaxris-compute-init` chowns that volume to UID `65532` before the API writes zips. Unpacked function/layer trees and invoke event scratch use other-readable modes (`0755` / `0644`) so nested DinD containers can bind-mount them; zip archives on disk stay `0600`. Compose sets `NOCTAXRIS_DOCKER_HOST=tcp://noctaxris-engine:2376` and `NOCTAXRIS_DOCKER_CERT_PATH=/certs/client`. The engine API stays on the Compose network only. Empty `NOCTAXRIS_DOCKER_HOST` disables DinD compute so unit tests can run without DinD. Without the engine, sync Invoke on the default DinD path returns compute unavailable. ImageUri pulls are allowlisted (lab ECR rewrite path plus pinned public Lambda bases). In-function SDK calls to `AWS_ENDPOINT_URL` / `NOCTAXRIS_LAMBDA_ENDPOINT_URL` (default `http://host.docker.internal:4566`) reach the published lab API without opening public internet egress.

### Nested compute

Compose starts `noctaxris-engine` (Docker-in-Docker) on the Compose network. Set `NOCTAXRIS_COMPUTE_RUNTIME=dind` or leave it unset. Unknown values fail process start. The path never falls through to host Docker. Prefer WSL or Linux with Docker Desktop for nested smoke; Windows-native hosts without a Linux VM cannot run DinD nested compute.

Image functions pull `ImageUri` inside DinD. Lab one-shot Invoke supports pinned AWS Lambda Python/Node/Java base tags (`public.ecr.aws/lambda/python:3.11`, `:3.12`, `:3.13`, `:3.14`, `nodejs:20`, `:22`, `:24`, `java:21`, `:25`, or the same path with `@sha256:...`), slim `python:` / `node:` refs matching those minors, and Temurin `eclipse-temurin:21-jdk` / `:25-jdk` (plus JRE pins). Zip Java Invoke prefers Temurin JDK images so the lab can `javac` a reflection bootstrap; Handler is `package.Class::method` or `package.Class` (default method `handleRequest`). Lab smoke handlers may use `public static String handleRequest(String in)`. Unrecognized ImageUri entrypoints fail closed unless the image is a lab ECR ref (full-container exec of the image ENTRYPOINT/CMD). For private lab images, push to the ECR lab registry ([ecr.md](ecr.md)) and reference `127.0.0.1:4566/ACCOUNT/REPO:tag` in `Code.ImageUri`. Invoke issues a registry token as the function execution role ARN and pulls with Registry V2 auth (role must Allow `ecr:BatchGetImage`).

`Environment.Variables` rejects reserved keys (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, `AWS_REGION`, `AWS_DEFAULT_REGION`, `AWS_LAMBDA_*`, lab `AWS_ENDPOINT_URL*`, and related runtime keys) with `InvalidParameterValueException`. Invoke applies function env first, then always overlays minted execution-role credentials and lab endpoint URLs. `Timeout` is clamped to 1–900 seconds; `MemorySize` to 128–10240 MB (mapped to the nested container memory limit). Zip Invoke uses a per-invoke scratch directory for the event payload and merged layer `/opt` tree.

### Authz notes

Lambda configure APIs use identity `EvaluateFull` plus `CheckPassRole` (caller `iam:PassRole` and Lambda service trust). Data-plane APIs including Invoke use `authorizeDataplaneOR` with the function owner account from the function ARN. Same-account access: allow if identity **or** function policy Allows. Cross-account access (Invoke with a full function ARN in another lab account): allow only when identity **and** function policy both Allow. Empty function policy denies cross-account callers. Explicit Deny wins. Boundary and SCP/RCP apply when identity Allows.

`AddPermission` accepts other lab account IAM user/role ARNs, account root ARNs, a 12-digit lab account id (stored as account root), or lab service principals (`sns.amazonaws.com`, `events.amazonaws.com`, `s3.amazonaws.com`, `apigateway.amazonaws.com`, and peers). Wildcard `*` principals require `FunctionUrlAuthType` (`NONE` or `AWS_IAM`). Optional `SourceAccount` / `SourceArn` persist as `Condition` (`aws:SourceAccount` StringEquals, `aws:SourceArn` ArnLike); `FunctionUrlAuthType` / `InvokedViaFunctionUrl` add `lambda:FunctionUrlAuthType` / `lambda:InvokedViaFunctionUrl`. Delivery and Function URL IAM invoke populate those keys so mismatched sources deny. `SourceAccount` may be the function account or another lab account (source resource owner), including for service-principal XA notify. `PrincipalOrgID` / `EventSourceToken` are not accepted (fail closed).

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

Node.js 24 zip create (CRUD; sync Invoke needs DinD like Python):

```bash
mkdir -p /tmp/noctaxris-lambda-node
cat > /tmp/noctaxris-lambda-node/index.js <<'JS'
exports.handler = async (event) => ({ ok: true, echo: event });
JS
(cd /tmp/noctaxris-lambda-node && zip -q /tmp/noctaxris-fn-node.zip index.js)

FN_NODE="noctaxris-node-$RANDOM"
aws lambda create-function \
  --function-name "$FN_NODE" \
  --runtime nodejs24.x \
  --role "$ROLE_ARN" \
  --handler index.handler \
  --zip-file fileb:///tmp/noctaxris-fn-node.zip \
  --endpoint-url "$EP"

aws lambda get-function --function-name "$FN_NODE" --endpoint-url "$EP" \
  --query 'Configuration.Runtime' --output text
```

Java 21 zip create. Handler is `package.Class::method` (or `package.Class` with default `handleRequest`). Lab one-shot Invoke compiles a reflection bootstrap with `javac` (Temurin JDK image preferred):

```bash
mkdir -p /tmp/noctaxris-lambda-java/example
cat > /tmp/noctaxris-lambda-java/example/Handler.java <<'JAVA'
package example;
public class Handler {
  public static String handleRequest(String in) {
    return "{\"ok\":true}";
  }
}
JAVA
(cd /tmp/noctaxris-lambda-java && zip -qr /tmp/noctaxris-fn-java.zip example)

FN_JAVA="noctaxris-java-$RANDOM"
aws lambda create-function \
  --function-name "$FN_JAVA" \
  --runtime java21 \
  --role "$ROLE_ARN" \
  --handler example.Handler::handleRequest \
  --zip-file fileb:///tmp/noctaxris-fn-java.zip \
  --endpoint-url "$EP"

aws lambda get-function --function-name "$FN_JAVA" --endpoint-url "$EP" \
  --query 'Configuration.[Runtime,Handler]' --output text
```

SDK coverage: Go/Node/Python suites create/get/delete every lab zip runtime (`tests/sdk/*/lambda*runtime*`). Terraform projects: `STACK=lab-lambda-python` (`python3.14`), `STACK=lab-lambda-nodejs` (`nodejs24.x`), `STACK=lab-lambda-java` (`java21` + `java25`) via `tests/terraform/run.sh` (or `TF_LAMBDA_RUNTIMES=1` / `NOCTAXRIS_ADVANCED=1` in `tests/run-all.sh`). Provider constraint for those stacks is `hashicorp/aws` `>= 6.21.0` so newer runtime enums validate.

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

Image CreateFunction (`PackageType=Image`). Handler and Runtime are optional (AWS). Lab one-shot Invoke supports AWS Lambda Python base images such as `public.ecr.aws/lambda/python:3.12`. When Handler is omitted on a pinned public base, Invoke uses a lab echo one-shot (`{"ok":true,"echo":...}`) because those bases have no `/var/task` app code. Pull happens inside DinD.

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
  --filter-criteria '{"Filters":[{"Pattern":"{\"body\":{\"status\":[\"ok\"]}}"}]}' \
  --endpoint-url "$EP"
```

Amazon MQ event source mapping. Create only when the broker is `RUNNING` with a nested non-`stub://` endpoint (see [mq.md](mq.md)). Skip without DinD / nested broker. Poll allowlists `noctaxris-mq-*` only. When the broker is `RUNNING`, RabbitMQ ESM Dial + `basic.get` on queue `noctaxris` (lab user `noctaxris` / `noctaxris-mq-lab`) returns real bodies; ActiveMQ remains dial-only empty (AMQP 1.0/JMS out of lab scope). Unit tests may inject `MQReceiveFunc`.

```bash
# BROKER_ARN from DescribeBroker when BrokerState=RUNNING
aws lambda create-event-source-mapping \
  --function-name "$FN" \
  --event-source-arn "$BROKER_ARN" \
  --batch-size 5 \
  --endpoint-url "$EP"
```

Function URL lite. AuthType `NONE` skips SigV4 on `http://127.0.0.1:4566/lambda-url/ACCOUNT/FUNCTION` and sets CORS (OPTIONS returns 204). Default allow-origin is `*`. Narrow with `Cors.AllowOrigins` on `CreateFunctionUrlConfig` or `NOCTAXRIS_FUNCTION_URL_CORS_ORIGINS` (comma-separated); then only a matching `Origin` is echoed. Treat open `*` as lab-only: any client that can reach the listener can invoke, and browsers can call cross-origin. On non-loopback listen (default Compose container bind), create and invoke require `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1`. AuthType `AWS_IAM` requires SigV4 service `lambda` plus `lambda:InvokeFunctionUrl`. For JWT-protected HTTP fronts, use API Gateway HTTP API rather than Function URLs. Lambda nested containers omit `host.docker.internal:host-gateway` by default; set `NOCTAXRIS_INJECT_HOST_GATEWAY=1` or use `docker/compose.lab-host-gateway.yaml` for in-function SDK calls to the host-published API. On Docker Desktop DinD, pair with `NOCTAXRIS_PUBLISH_ADDR=0.0.0.0` for that session if loopback publish is unreachable via host-gateway. ECS / CodeBuild / Batch tasks use a separate opt-in (`NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1`).

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

## Out of lab scope

- Full Lambda SAR (provisioned concurrency, weighted alias routing, SnapStart, VPC ENI, recursive loop protection depth, tags, tracing, code signing) (out of lab scope; zip/Image/ESM/URLs cover marketed compute)
- EventBridge or Lambda-to-Lambda failure destinations (out of lab scope; OnFailure to SQS and SNS is shipped)
- Kinesis Enhanced Fan-Out (`RegisterStreamConsumer` / `SubscribeToShard`) / `ParallelizationFactor` (out of lab scope; sequential multi-shard ESM is the lab path)
- FilterCriteria `$or` / `wildcard` / `cidr`, FilterCriteria KMS encryption (`KMSKeyArn`), and Kafka/MQ filter shapes (out of lab scope)
- ActiveMQ AMQP 1.0/JMS consumer for MQ ESM (lab limit: allowlisted TCP dial then empty batch; RabbitMQ AMQP 0-9-1 `basic.get` is shipped when the nested broker is `RUNNING`)
- Function URL CORS beyond AllowOrigins allowlist (methods/headers/MaxAge depth) (out of lab scope). Prefer API Gateway HTTP API for JWT labs. CloudFront is a config stub only (see [cloudfront.md](cloudfront.md))
- Fully rootless nested engine (out of lab scope; default Compose already uses restricted DinD without `privileged: true`; privileged opt-in is `compose.engine-privileged.yaml` — see [security-defaults.md](../security-defaults.md))

## Will not ship

- Non-lab private registries (Docker Hub private, third-party hosts) for Lambda Image (will not ship; needs egress beyond lab ECR). Lab ECR on `127.0.0.1:4566` remains supported for Image Invoke
