# AppSync

**Status:** shipped (lab core)

GraphQL API CRUD lite, schema store, Lambda data source and one Query field resolver, minimal GraphQL query evaluation that Invokes Lambda. No GraphQL library dependency (field stub parser).

## Implemented

| Area | Actions |
|------|---------|
| API | `CreateGraphqlApi`, `GetGraphqlApi`, `ListGraphqlApis`, `DeleteGraphqlApi` |
| Schema | `StartSchemaCreation` (stores SDL immediately) |
| Auth prep | `CreateApiKey` (API_KEY APIs only) |
| Data plane | `CreateDataSource` (AWS_LAMBDA), `CreateResolver` |
| Runtime | `POST /appsync/{apiId}/graphql` |

### Auth shape (v5)

| authenticationType | Runtime auth | Notes |
|--------------------|--------------|-------|
| `API_KEY` | Header `x-api-key` | CreateApiKey returns the key in `apiKey.id` |
| `AWS_IAM` | SigV4 service `appsync` + `appsync:GraphQL` | Unsigned requests rejected |
| Cognito / OIDC / Lambda authorizer | Not accepted | Deferred to v6 with Cognito |

### Authz notes

Identity `EvaluateFull` on management `appsync:*`. GraphQL IAM path also checks `appsync:GraphQL` on the API ARN.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws appsync create-graphql-api --name lab --authentication-type API_KEY --endpoint-url "$EP"
aws appsync start-schema-creation --api-id "$API" --definition 'type Query { hello: String }' --endpoint-url "$EP"
aws appsync create-api-key --api-id "$API" --endpoint-url "$EP"
aws appsync create-data-source --api-id "$API" --name HelloDS --type AWS_LAMBDA \
  --lambda-config lambdaFunctionArn=arn:aws:lambda:us-east-1:000000000001:function:hello --endpoint-url "$EP"
aws appsync create-resolver --api-id "$API" --type-name Query --field-name hello --data-source-name HelloDS --endpoint-url "$EP"
curl -s -H "x-api-key: $KEY" -H "content-type: application/json" \
  -d '{"query":"{ hello }"}' "http://127.0.0.1:4566/appsync/$API/graphql"
```

GraphQL Invoke requires a registered Lambda function and DinD compute. Document skip when Docker is unavailable.

## Not yet / deferred

- Cognito User Pools authorizer (v6)
- Amplify, subscriptions/MQTT, full GraphQL spec
- AppSync JS/VTL runtimes beyond Lambda Invoke for one Query field
