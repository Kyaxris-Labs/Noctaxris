# AppSync

**Status:** shipped (lab core)

GraphQL API CRUD lite, schema store with creation status, Lambda data sources with optional `serviceRoleArn`, Query and nested object field resolvers, management SAR (API keys / data sources / resolvers), and minimal GraphQL evaluation that Invokes Lambda. No GraphQL library dependency (selection-set parser with nesting depth limit 3).

## Implemented

| Area | Actions |
|------|---------|
| API | `CreateGraphqlApi`, `GetGraphqlApi`, `ListGraphqlApis`, `DeleteGraphqlApi` |
| Schema | `StartSchemaCreation` (stores SDL immediately), `GetSchemaCreationStatus` (`NOT_APPLICABLE` until start; `SUCCESS` after) |
| Auth prep | `CreateApiKey`, `ListApiKeys`, `DeleteApiKey` (API_KEY APIs only) |
| Data plane | `Create`/`Update`/`Get`/`List`/`DeleteDataSource` (AWS_LAMBDA, optional `serviceRoleArn`); `Create`/`Update`/`Get`/`List`/`DeleteResolver` (`typeName` + `fieldName`) |
| Runtime | `POST /appsync/{apiId}/graphql` (multi-field Query + nested selections) |

### Auth shape

| authenticationType | Runtime auth | Notes |
|--------------------|--------------|-------|
| `API_KEY` | Header `x-api-key` | CreateApiKey returns the plaintext key once as `apiKey.id`; only an HMAC-SHA256 hash (master key) is stored at rest. ListApiKeys returns opaque row UUIDs (not recoverable secrets). DeleteApiKey accepts the Create plaintext or a List UUID. |
| `AWS_IAM` | SigV4 service `appsync` + `appsync:GraphQL` | Unsigned requests rejected |
| `AMAZON_COGNITO_USER_POOLS` | Bearer JWT | `userPoolConfig` with `userPoolId`, `clientId` (audience), optional `issuer` (lab Cognito shape by default). Verifies via shared jose helper against lab Cognito JWKS. Requires `token_use=id`. Enforces `exp` and `nbf`. Non-lab issuers require `NOCTAXRIS_ALLOW_REMOTE_JWKS` |

### Authz notes

Identity `EvaluateFull` on management `appsync:*`. GraphQL IAM path requires SigV4 service `appsync` and `appsync:GraphQL` on the API ARN.

Lambda data-source invoke has two lab paths:

| Create/UpdateDataSource | Invoke authorization |
|-------------------------|----------------------|
| No `serviceRoleArn` | Resource policy only: function must Allow `appsync.amazonaws.com` (`lambda:AddPermission`) |
| `serviceRoleArn` set | Configure-time PassRole + `appsync.amazonaws.com` trust (API ARN as `aws:SourceArn`); invoke mints a role session and requires `lambda:InvokeFunction` Allow on that session |

`DeleteDataSource` fails closed when resolvers still reference the data source. `ListResolvers` requires `typeName`.

### GraphQL runtime limits

- Selection sets with nesting up to depth 3, e.g. `{ getUser { name email } }` or `{ a { b { c } } }`
- Deeper nesting (`{ a { b { c { d } } } }`), field arguments, aliases, and fragments fail closed
- Each field with a `CreateResolver` entry Invokes its Lambda; nested fields use `typeName` from the parent field's schema return type
- Fields without a unit resolver project from the parent JSON object (GraphQL default field resolver); missing keys become `null`
- Nested Lambda events include `source` (parent object) and `info.parentTypeName`
- List parents: nested selections apply per list item when the parent value is a JSON array of objects
- Unknown root fields return a GraphQL error entry for that field (partial `data` + `errors`)
- No subscriptions; Mutation multi-field is not a separate path (operation prefix stripped; root resolvers are Query-typed)
- No AppSync JS/VTL mapping templates (Lambda Invoke only)

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

Resource-policy path (flat + nested):

```bash
aws lambda add-permission --function-name get-user --statement-id appsync \
  --action lambda:InvokeFunction --principal appsync.amazonaws.com --endpoint-url "$EP"
aws lambda add-permission --function-name user-email --statement-id appsync \
  --action lambda:InvokeFunction --principal appsync.amazonaws.com --endpoint-url "$EP"
aws appsync create-graphql-api --name lab --authentication-type API_KEY --endpoint-url "$EP"
aws appsync start-schema-creation --api-id "$API" --definition \
  'type Query { getUser: User } type User { name: String email: String }' --endpoint-url "$EP"
aws appsync get-schema-creation-status --api-id "$API" --endpoint-url "$EP"
aws appsync create-api-key --api-id "$API" --endpoint-url "$EP"
aws appsync list-api-keys --api-id "$API" --endpoint-url "$EP"
aws appsync create-data-source --api-id "$API" --name GetUserDS --type AWS_LAMBDA \
  --lambda-config lambdaFunctionArn=arn:aws:lambda:us-east-1:000000000001:function:get-user --endpoint-url "$EP"
aws appsync list-data-sources --api-id "$API" --endpoint-url "$EP"
aws appsync get-data-source --api-id "$API" --name GetUserDS --endpoint-url "$EP"
aws appsync create-data-source --api-id "$API" --name EmailDS --type AWS_LAMBDA \
  --lambda-config lambdaFunctionArn=arn:aws:lambda:us-east-1:000000000001:function:user-email --endpoint-url "$EP"
aws appsync create-resolver --api-id "$API" --type-name Query --field-name getUser \
  --data-source-name GetUserDS --endpoint-url "$EP"
aws appsync list-resolvers --api-id "$API" --type-name Query --endpoint-url "$EP"
aws appsync get-resolver --api-id "$API" --type-name Query --field-name getUser --endpoint-url "$EP"
aws appsync create-resolver --api-id "$API" --type-name User --field-name email \
  --data-source-name EmailDS --endpoint-url "$EP"
curl -s -H "x-api-key: $KEY" -H "content-type: application/json" \
  -d '{"query":"{ getUser { name email } }"}' "http://127.0.0.1:4566/appsync/$API/graphql"
```

Flat multi-field Query still works: `{ hello world }` Invokes each Query resolver independently. Nested `{ getUser { name email } }` Invokes `Query.getUser`, projects `name` from the parent object when no `User.name` resolver exists, and Invokes `User.email` when that resolver is registered.

PassRole path: create an IAM role trusted by `appsync.amazonaws.com` with `lambda:InvokeFunction`, then pass `--service-role-arn` on `create-data-source` / `update-data-source` (resource policy on the function is not required for same-account role-session invoke). Nested field resolvers use the same PassRole / resource-policy rules per data source.

Cognito auth example: create the API with `--authentication-type AMAZON_COGNITO_USER_POOLS` and a `userPoolConfig`, obtain an **IdToken** from Cognito `InitiateAuth`, then:

```bash
curl -s -H "Authorization: Bearer $ID_TOKEN" -H "content-type: application/json" \
  -d '{"query":"{ hello }"}' "http://127.0.0.1:4566/appsync/$API/graphql"
```

GraphQL Invoke requires a registered Lambda function and DinD compute (or a test invoke hook). Document skip when Docker is unavailable.

## Not yet / deferred

- Amplify, subscriptions/MQTT, full GraphQL spec
- Aliases, fragments, field arguments, selection depth beyond 3
- AppSync JS/VTL mapping template runtimes
- OIDC providers beyond Cognito User Pools, Lambda authorizer
