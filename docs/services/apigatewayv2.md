# API Gateway HTTP API

**Status:** shipped (lab core)

HTTP API (API Gateway v2) lite: CreateApi / CreateIntegration / CreateAuthorizer / CreateRoute / CreateStage, Lambda AWS_PROXY invoke. JWT and IAM authorizers. No REST API v1. No HTTP_PROXY to arbitrary URLs.

## Implemented

| Area | Actions |
|------|---------|
| API | `CreateApi` (ProtocolType HTTP), `GetApi`, `GetApis`, `DeleteApi` |
| Integration | `CreateIntegration` (AWS_PROXY Lambda ARN only, optional `CredentialsArn` with PassRole) |
| Authorizer | `CreateAuthorizer` (JWT: Issuer + Audience) |
| Route | `CreateRoute` (AuthorizationType NONE, JWT, or AWS_IAM) |
| Stage | `CreateStage` (`$default` common) |
| Invoke | `GET/POST http://127.0.0.1:4566/http-api/{apiId}/{stage}/{path}` |

### Route auth

| AuthorizationType | Runtime | Notes |
|-------------------|---------|-------|
| `NONE` | Open on loopback | Lab open path |
| `JWT` | Bearer token | Verifies Cognito (or other) JWKS via shared jose helper. Prefer access token (`token_use=access`). Audience matches `aud` or `client_id` |
| `AWS_IAM` | SigV4 service `execute-api` | Identity `execute-api:Invoke` on route ARN. HTTP API resource policies are not supported |

Management APIs use SigV4 service `apigateway` and `apigatewayv2:*` identity actions.

### Authz notes

Identity `EvaluateFull` on manage actions. IAM invoke uses `execute-api:Invoke` only (no invent HTTP API resource policy).

When `CredentialsArn` is set on `CreateIntegration`, PassRole plus `apigateway.amazonaws.com` trust is required.

Lambda invoke from Gateway follows the same nested compute path as Function URLs. Grant `lambda:AddPermission` for `apigateway.amazonaws.com` in labs when you want policy fidelity. The lab invoke path does not require a resource policy statement.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws apigatewayv2 create-api --name lab --protocol-type HTTP --endpoint-url "$EP"
aws apigatewayv2 create-integration --api-id "$API" --integration-type AWS_PROXY \
  --integration-uri "arn:aws:lambda:us-east-1:000000000001:function:hello" --endpoint-url "$EP"
aws apigatewayv2 create-route --api-id "$API" --route-key "GET /hello" \
  --target "integrations/$INT" --authorization-type NONE --endpoint-url "$EP"
aws apigatewayv2 create-stage --api-id "$API" --stage-name '$default' --endpoint-url "$EP"
curl -s "http://127.0.0.1:4566/http-api/$API/\$default/hello"
```

JWT lab: create a Cognito user pool and client, `InitiateAuth`, create a JWT authorizer with Issuer `http://127.0.0.1:4566/cognito-idp/us-east-1/$POOL` and Audience client id, then call the route with `Authorization: Bearer $ACCESS_TOKEN`.

Invoke requires a registered Lambda and DinD compute. Document skip when Docker is unavailable.

## Not yet / deferred

- REST API (v1) and WebSocket APIs
- HTTP_PROXY / VPC link integrations
- Lambda authorizers
- Custom domains beyond lab ACM string linkage
- CORS configuration depth
