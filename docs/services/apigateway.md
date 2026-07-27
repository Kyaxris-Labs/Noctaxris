# API Gateway REST API and WebSocket

**Status:** shipped (lab core for REST; lab lite for WebSocket)

REST API (API Gateway v1) lite: CreateRestApi / resource tree / PutMethod / PutIntegration (AWS_PROXY Lambda + MOCK) / CreateDeployment / CreateStage, Floci-shaped execute path. Method auth NONE or AWS_IAM.

WebSocket APIs (v2 ProtocolType WEBSOCKET) lite: `$connect` / `$disconnect` / `$default` AWS_PROXY routes, in-memory connections, HTTP lab stand-in (not a real `ws://` upgrade).

HTTP API remains under [apigatewayv2.md](apigatewayv2.md).

## REST API Implemented

| Area | Actions |
|------|---------|
| API | `CreateRestApi`, `GetRestApi`, `GetRestApis`, `DeleteRestApi` |
| Resource | `CreateResource`, `GetResources`, `DeleteResource` (root cannot be deleted) |
| Method | `PutMethod`, `GetMethod`, `DeleteMethod` (`authorizationType` NONE or AWS_IAM) |
| Integration | `PutIntegration` / `GetIntegration` (`AWS_PROXY` Lambda, `MOCK`, or opt-in `HTTP_PROXY` / `VPC_LINK`) |
| Deploy | `CreateDeployment` (optional `stageName` creates/updates stage), `CreateStage`, `GetStage` |
| Invoke | `GET/POST http://127.0.0.1:4566/restapis/{apiId}/{stage}/_user_request_/{path}` |

Management uses SigV4 service `apigateway` on `/restapis/...` (not `/v2/apis`). Execute is data-plane and does not require management SigV4.

### Method auth

| authorizationType | Runtime | Notes |
|-------------------|---------|-------|
| `NONE` | Open when listen is loopback, or with `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1` | Same open-data-plane gate as HTTP API |
| `AWS_IAM` | SigV4 service `execute-api` | Identity `execute-api:Invoke` on method ARN |

### Integrations

| type | Lab support |
|------|-------------|
| `MOCK` | Returns HTTP 200; body from `requestTemplates["application/json"]` when set, else `{"message":"OK"}` |
| `AWS_PROXY` | Lambda ARN or `arn:aws:apigateway:region:lambda:path/2015-03-31/functions/.../invocations`. Payload format is REST proxy 1.0. Without nested DinD, invoke returns 503 |
| `HTTP_PROXY` / `VPC_LINK` | Opt-in: `NOCTAXRIS_APIGW_HTTP_PROXY=1` plus `NOCTAXRIS_APIGW_HTTP_PROXY_ALLOWLIST` (hosts or URL prefixes). Link-local/metadata/private need an allowlist entry naming that host. No redirect follow; pinned DialContext |

Without `credentials` on AWS_PROXY, invoke requires a Lambda resource policy Allow for `apigateway.amazonaws.com` (`lambda:AddPermission`).

## HTTP_PROXY / VPC_LINK (opt-in)

Default deny on REST and HTTP API. See [configuration.md](../configuration.md) and [security-defaults.md](../security-defaults.md).

## REST How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
# NONE methods need loopback listen, or NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1 under Compose.
API_ID=$(aws apigateway create-rest-api --name lab-rest --query id --output text --endpoint-url "$EP")
ROOT_ID=$(aws apigateway get-resources --rest-api-id "$API_ID" \
  --query 'items[?path==`/`].id' --output text --endpoint-url "$EP")
RES_ID=$(aws apigateway create-resource --rest-api-id "$API_ID" --parent-id "$ROOT_ID" \
  --path-part hello --query id --output text --endpoint-url "$EP")
aws apigateway put-method --rest-api-id "$API_ID" --resource-id "$RES_ID" \
  --http-method GET --authorization-type NONE --endpoint-url "$EP"
aws apigateway put-integration --rest-api-id "$API_ID" --resource-id "$RES_ID" \
  --http-method GET --type MOCK \
  --request-templates '{"application/json":"{\"message\":\"ok\"}"}' --endpoint-url "$EP"
aws apigateway create-deployment --rest-api-id "$API_ID" --stage-name dev --endpoint-url "$EP"
curl -s "http://127.0.0.1:4566/restapis/$API_ID/dev/_user_request_/hello"
```

Lambda AWS_PROXY (needs registered function + DinD; soft-skip when compute unavailable):

```bash
aws lambda add-permission --function-name hello --statement-id apigw-rest \
  --action lambda:InvokeFunction --principal apigateway.amazonaws.com --endpoint-url "$EP"
aws apigateway put-integration --rest-api-id "$API_ID" --resource-id "$RES_ID" \
  --http-method GET --type AWS_PROXY --integration-http-method POST \
  --uri "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/arn:aws:lambda:us-east-1:000000000001:function:hello/invocations" \
  --endpoint-url "$EP"
```

## WebSocket Implemented

| Area | Support |
|------|---------|
| API | `CreateApi` with `ProtocolType=WEBSOCKET` (apigatewayv2 management under `/v2/apis`) |
| Routes | `$connect`, `$disconnect`, `$default` → AWS_PROXY Lambda |
| Lab invoke | `POST http://127.0.0.1:4566/ws-api/{apiId}/{stage}/$connect|$disconnect|$default` |
| Management | `PostToConnection` via `POST /execute-api/{apiId}/{stage}/@connections/{connectionId}` (in-memory connection table) |

Not a full internet WebSocket gateway: no `ws://` upgrade on `:4566`. Soft-skip Lambda invoke when nested compute is down.

### WebSocket CLI smoke

```bash
aws apigatewayv2 create-api --name lab-ws --protocol-type WEBSOCKET \
  --route-selection-expression '$request.body.action' --endpoint-url "$EP"
# create integration + $connect/$disconnect/$default routes + stage, then:
curl -s -X POST "http://127.0.0.1:4566/ws-api/$API/\$default/\$connect"
```

## Deferred depth

- REQUEST / TOKEN authorizers, usage plans, API keys, models, validators, gateway responses
- Method / integration response maps beyond MOCK default body
- Import/export OpenAPI, custom domains, base path mappings
- Real `ws://` upgrade and full connection fan-out

## Out of lab scope

- Full AWS REST feature parity (documentation parts, client certificates, canary stages)
- Invented HTTP API-style resource policies for REST (IAM invoke via `execute-api:Invoke` only)

## Will not ship (default)

- Open `HTTP_PROXY` / VPC link without `NOCTAXRIS_APIGW_HTTP_PROXY=1` and allowlist (will not ship; open SSRF class)
