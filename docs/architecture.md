# Architecture (Phase 7)

One Go binary runs in the API container. There is no sidecar API gateway and no host Docker socket mount. Compose also starts `noctaxris-engine` (Docker-in-Docker) on the Compose network so Lambda Invoke can run nested function containers.

## Overview

Host publish is loopback only. Nested compute and data engines talk to `noctaxris-engine` over TLS on the Compose network. The API never mounts host `docker.sock`.

```mermaid
flowchart TB
  Client["AWS CLI / SDK"]
  HostPort["127.0.0.1:4566"]

  subgraph Compose["Compose project"]
    API["noctaxris API container<br/>no host docker.sock"]
    Engine["noctaxris-engine<br/>DinD TLS :2376<br/>not published to host"]
  end

  subgraph Nested["Inside noctaxris-engine"]
    FnNet["Lambda on noctaxris-fn<br/>Internal network"]
    EcsNet["ECS / CodeBuild / Batch<br/>on noctaxris-ecs Internal"]
    DataNet["RDS / ElastiCache / DocDB<br/>nested-network endpoints only"]
  end

  MicroVM["Opt-in microVM<br/>NOCTAXRIS_COMPUTE_RUNTIME=microvm<br/>Linux + KVM + Firecracker<br/>fail-closed otherwise"]

  Client --> HostPort --> API
  API -->|"TCP TLS NOCTAXRIS_DOCKER_HOST"| Engine
  Engine --> FnNet
  Engine --> EcsNet
  Engine --> DataNet
  API -.->|"runtime selection"| MicroVM
```

## Tree

```text
Noctaxris/
  cmd/noctaxris/
  docker/                         # API image + compose (noctaxris + noctaxris-engine)
  internal/config/
  internal/compute/               # nested DinD client, Internal network, one-shot Invoke
  internal/store/                 # IAM, KMS, S3, DynamoDB, SQS, Lambda, Cognito, Gateway, orgs, IdP, ...
  internal/catalog/
  internal/kernel/audit/
  internal/kernel/authn/          # SigV4 header + query (presign)
  internal/kernel/authz/          # Evaluate*, CheckPassRole
  internal/kernel/federation/     # OIDC/SAML federation verify (go-jose v4)
  internal/kernel/jwtutil/        # shared RS256 JWKS issue/verify helper
  internal/kernel/identity/
  internal/kernel/sts/
  internal/services/
  internal/server/
  docs/                           # public docs
    docs/services/                # per-service APIs, smoke, deferred depth
```

## Request path

```text
HTTP request
  ├─ GET /_noctaxris/health → 200 ok
  ├─ GET /cognito-idp/{region}/{pool}/.well-known/jwks.json → JWKS (no SigV4)
  ├─ /http-api/{apiId}/{stage}/{path} → Gateway invoke (NONE / JWT / IAM)
  ├─ /lambda-url/{account}/{function} → Function URL invoke
  ├─ /appsync/{apiId}/graphql → AppSync GraphQL (API_KEY / IAM / Cognito)
  └─ else
       ├─ AssumeRoleWithSAML / AssumeRoleWithWebIdentity → IdP crypto (no SigV4)
       ├─ else authn.Verify (SigV4 header or query)
       ├─ if S3 path-style REST (service s3 / empty Action) → EvaluateS3 then handler
       ├─ GetCallerIdentity → XML (no IAM Evaluate)
       ├─ DynamoDB / SQS → EvaluateDynamoDB / EvaluateSQS then handler
       ├─ Lambda → identity or resource policy (dataplane), PassRole on role configure, then handler
       │    └─ Invoke → mint execution-role session, compute.RunInvoke via noctaxris-engine (TLS)
       ├─ Cognito / API Gateway / AppSync / edge and governance services → identity EvaluateFull then handler
       ├─ other STS / IAM / KMS / Organizations → existing Evaluate paths
       └─ unknown → 501 NotImplemented
```

Object bytes live under `$DATAROOT/s3/{account}/{bucket}/...`. Lambda zip contents live under `$DATAROOT/lambda/...` and are shared with DinD through the Compose data volume. Bucket metadata, object metadata (etag, SSE), DynamoDB tables/items, SQS queues/messages, and Lambda function metadata live in SQLite. Cognito signing keys are sealed under the store master key. BCM export samples land under `$DATAROOT/bcm-exports/...`.

## Compute path

Compose sets `NOCTAXRIS_DOCKER_HOST=tcp://noctaxris-engine:2376` and `NOCTAXRIS_DOCKER_CERT_PATH=/certs/client` for TLS to the nested engine. The API process never mounts host `/var/run/docker.sock`. `noctaxris-engine` is privileged DinD so nested containers can start. The engine API is not published to the host. Function containers attach to DinD network `noctaxris-fn` with `Internal: true` (no public internet route by default). Empty `NOCTAXRIS_DOCKER_HOST` disables compute so unit tests can run without DinD.

Default Lambda and ECS compute runtime is DinD (`NOCTAXRIS_COMPUTE_RUNTIME` unset or `dind`). Opt-in `microvm` selects a Firecracker-class path on Linux with usable `/dev/kvm` and a Firecracker binary. WSL2 stays DinD-only. Missing KVM or binary fails closed without falling through to host Docker. Live guest zip/Image Invoke and ECS RunTask still require kernel/rootfs assets on a Linux+KVM host. CodeBuild and Batch stay on the DinD path.

## Nested data planes

RDS, ElastiCache, and DocumentDB engine processes (when started) are nested containers via the same `noctaxris-engine` TLS client used for Lambda. Labels such as `noctaxris.data=rds|elasticache|docdb` identify them. Host Compose still publishes only `127.0.0.1:4566`.

```mermaid
flowchart TD
  Create["CreateDBInstance / CreateCacheCluster / CreateDBCluster"]
  Store["store row<br/>nested-network endpoint, secret ARN, status"]
  Helper["data-plane helper<br/>compute.Client DinD TLS"]
  Start["start labeled nested container<br/>no host port publish"]
  Describe["Describe* returns nested-network hostname:port"]
  DataAPI["RDS Data API ExecuteStatement on :4566<br/>stub executor by default"]

  Create --> Store --> Helper --> Start --> Describe
  Helper -.-> DataAPI
```

Athena queries Glue catalog metadata and lab S3 object bytes **in-process** on the API (no nested query engine required). OpenSearch domain CRUD returns a loopback stub endpoint (MQ-style). Live nested OpenSearch is optional and must not publish search ports on the host.

When DinD is unset, create paths keep control-plane rows and nested start is a no-op. Live engine start requires `noctaxris-engine`. Do not mount the operator host filesystem into nested data containers.

## In-process delivery workers

- EventBridge Scheduler advances due schedules inside the API process and delivers via existing Lambda async enqueue, SQS SendMessage, and SNS Publish helpers.
- Lambda SQS event source mappings poll with ReceiveMessage, synchronously Invoke, and DeleteMessage on success.
- EventBridge Pipes reuse the same poll and invoke/send helpers for SQS and DynamoDB Streams sources.
- SNS HTTP(S) subscriptions deliver only to allowlisted loopback endpoints (lab catcher). Non-allowlisted URLs fail closed.

## Edge identity

- Cognito issues RS256 ID and access tokens and serves JWKS on the same `:4566` listener. Issuer shape: `http://127.0.0.1:4566/cognito-idp/<region>/<userPoolId>`.
- API Gateway HTTP API JWT authorizer verifies Bearer tokens via the shared jose helper against lab Cognito JWKS. IAM routes require SigV4 and `execute-api:Invoke` (no HTTP API resource policies).
- AppSync accepts `AMAZON_COGNITO_USER_POOLS` beside API_KEY and AWS_IAM.
- Gateway `CreateIntegration` optional `CredentialsArn` enforces PassRole plus `apigateway.amazonaws.com` trust.
- CloudFront and ELBv2 are config-shaped stubs (no real PoP, no EC2 targets). Gateway must not open HTTP_PROXY to arbitrary URLs.

## Authz

- Same-account identity: `Evaluate` / `EvaluateFull` (identity, boundary, SCP, RCP)
- Resource-policy dataplane (S3, SQS, Lambda, ECR, SNS, Secrets, DynamoDB): same-account Allow if identity **or** resource policy Allows (Deny-overrides). Cross-account Allow only when identity **and** resource policy both Allow. Empty resource policy denies cross-account callers.
- KMS: `EvaluateKMS`. Key-policy explicit Allow (or grant) is always required. Cross-account also needs caller identity Allow.
- Organizations SCP/RCP: collected from account attachments, OU path to root, and root. Management account exempt.
- Lambda configure: `CheckPassRole` (caller `iam:PassRole` plus `lambda.amazonaws.com` trust)
- Session policies: `EvaluateWithSession` intersection
- Cross-account AssumeRole: `EvaluateCrossAccount` (trust dual-eval, separate from resource dual-eval)
- Condition request context: populates cataloged keys such as `aws:SourceIp`, `aws:PrincipalArn`, and ResourceTag keys when present

## Identity and data

- Env-injected keys are management account root
- IAM user secrets and KMS CMK material sealed at rest
- S3 SSE-S3 DEKs sealed with the data-root master key
- SSE-KMS uses Phase 4 GenerateDataKey / Decrypt under the caller
- DynamoDB item ciphertext uses table SSE (AWS-owned or customer-managed KMS)
- SQS message bodies use SSE-SQS or SSE-KMS when queue attributes request encryption
- Lambda Invoke injects temporary AWS_* credentials for the function execution role
