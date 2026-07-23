# API Gateway HTTP API

**Status:** shipped (lab core)

HTTP API (API Gateway v2) lite: CreateApi / CreateIntegration / CreateAuthorizer / CreateRoute / CreateStage, Lambda AWS_PROXY invoke. JWT, IAM, and Lambda authorizers. No REST API v1. No HTTP_PROXY to arbitrary URLs.

## Implemented

| Area | Actions |
|------|---------|
| API | `CreateApi` (ProtocolType HTTP), `GetApi`, `GetApis`, `DeleteApi` |
| Integration | `CreateIntegration` (AWS_PROXY Lambda ARN only, optional `CredentialsArn` with PassRole), `GetIntegrations` |
| Authorizer | `CreateAuthorizer` (JWT: Issuer + Audience; REQUEST Lambda authorizer), `GetAuthorizers` |
| Route | `CreateRoute` (AuthorizationType NONE, JWT, AWS_IAM, or CUSTOM), `GetRoutes` |
| Stage | `CreateStage` (`$default` common) |
| REST vs Registry | HTTP API management REST under `/v2/apis...` is matched before lab ECR Docker Registry `/v2/` on the same listener |
| Invoke | `GET/POST http://127.0.0.1:4566/http-api/{apiId}/{stage}/{path}` |

### Route auth

| AuthorizationType | Runtime | Notes |
|-------------------|---------|-------|
| `NONE` | Open when listen is loopback, or with `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1` | Create and invoke refuse `NONE` on non-loopback without the opt-in. Default Compose does not set the opt-in (container bind is non-loopback) |
| `JWT` | Bearer token | Verifies lab Cognito JWKS in-process (issuer `http://127.0.0.1:4566/cognito-idp/...`). Remote JWKS issuers fail closed unless `NOCTAXRIS_ALLOW_REMOTE_JWKS=1` with a public host allowlist. Requires `token_use=access` (rejects missing or `id`). Enforces `exp` and `nbf`. Audience matches `aud` or `client_id`. `IdentitySource` must be `$request.header.Authorization` |
| `AWS_IAM` | SigV4 service `execute-api` | Identity `execute-api:Invoke` on route ARN. HTTP API resource policies are not supported |
| `CUSTOM` | Lambda authorizer (REQUEST) | Invokes the authorizer Lambda before the integration. Deny returns 403 and skips integration. See Lambda authorizer below |

Management APIs use SigV4 service `apigateway` and `apigatewayv2:*` identity actions.

### Lambda authorizer

HTTP API `AuthorizerType=REQUEST` only. REST-style `TOKEN` is rejected.

| Field | Lab support |
|-------|-------------|
| `AuthorizerUri` | Lambda ARN, or `arn:aws:apigateway:region:lambda:path/2015-03-31/functions/.../invocations` |
| `AuthorizerPayloadFormatVersion` | `1.0` or `2.0` (default `2.0`) |
| `EnableSimpleResponses` | Default true for simple `{ "isAuthorized": true\|false }` |
| `AuthorizerCredentialsArn` | Optional; PassRole + `apigateway.amazonaws.com` trust at create; role-session `lambda:InvokeFunction` at invoke |
| `IdentitySource` | `$request.header.*`, `$request.querystring.*`, or `$context.routeKey` |

Supported authorizer responses (fail closed otherwise):

- Simple: `{ "isAuthorized": true|false }` (optional `context`)
- IAM policy: `policyDocument.Statement[].Effect` Allow/Deny (any Deny or missing Allow denies)

Without `AuthorizerCredentialsArn`, authorizer invoke requires a Lambda resource policy Allow for `apigateway.amazonaws.com` (same parity as AWS_PROXY integration). Missing permission returns 403 before the authorizer runs.

Proxy integration responses strip hop-by-hop headers and `Set-Cookie` by default. Set `NOCTAXRIS_HTTP_API_ALLOW_SET_COOKIE=1` to pass `Set-Cookie` through.

### Authz notes

Identity `EvaluateFull` on manage actions. IAM invoke uses `execute-api:Invoke` only (no invent HTTP API resource policy).

When `CredentialsArn` is set on `CreateIntegration`, PassRole plus `apigateway.amazonaws.com` trust is required. At invoke, a role session for that ARN must Allow `lambda:InvokeFunction` on the integration target.

Without `CredentialsArn`, invoke requires a Lambda resource policy Allow for `apigateway.amazonaws.com` (`lambda:AddPermission`; optional `SourceArn` of the execute-api route). Missing permission returns 403.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
# NONE routes need loopback listen, or NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1 under Compose.
aws lambda add-permission --function-name hello --statement-id apigw \
  --action lambda:InvokeFunction --principal apigateway.amazonaws.com --endpoint-url "$EP"
aws apigatewayv2 create-api --name lab --protocol-type HTTP --endpoint-url "$EP"
aws apigatewayv2 create-integration --api-id "$API" --integration-type AWS_PROXY \
  --integration-uri "arn:aws:lambda:us-east-1:000000000001:function:hello" --endpoint-url "$EP"
aws apigatewayv2 create-route --api-id "$API" --route-key "GET /hello" \
  --target "integrations/$INT" --authorization-type NONE --endpoint-url "$EP"
aws apigatewayv2 create-stage --api-id "$API" --stage-name '$default' --endpoint-url "$EP"
curl -s "http://127.0.0.1:4566/http-api/$API/\$default/hello"
```

JWT lab: create a Cognito user pool and client, `InitiateAuth`, create a JWT authorizer with Issuer `http://127.0.0.1:4566/cognito-idp/us-east-1/$POOL` and Audience client id, then call the route with `Authorization: Bearer $ACCESS_TOKEN`.

Lambda authorizer lab: create an authorizer function that returns `{ "isAuthorized": true }`, `add-permission` for `apigateway.amazonaws.com` on that function, then:

```bash
aws apigatewayv2 create-authorizer --api-id "$API" --name lab-authz --authorizer-type REQUEST \
  --authorizer-uri "arn:aws:lambda:us-east-1:000000000001:function:authz" \
  --authorizer-payload-format-version 2.0 --enable-simple-responses \
  --identity-source '$request.header.Authorization' --endpoint-url "$EP"
aws apigatewayv2 create-route --api-id "$API" --route-key "GET /secure" \
  --target "integrations/$INT" --authorization-type CUSTOM --authorizer-id "$AUTHZ" --endpoint-url "$EP"
```

Invoke requires a registered Lambda and DinD compute. Document skip when Docker is unavailable.

## Not yet / deferred

- REST API (v1) and WebSocket APIs
- REST TOKEN authorizers (HTTP API REQUEST only)
- HTTP_PROXY / VPC link integrations
- Authorizer result caching / TTL
- Custom domains beyond lab ACM string linkage
- CORS configuration depth on HTTP API itself
