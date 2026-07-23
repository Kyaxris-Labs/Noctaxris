# Architecture

One Go binary runs in the API container. There is no sidecar API gateway and no host Docker socket mount. Compose also starts `noctaxris-engine` (Docker-in-Docker) on the Compose network so Lambda Invoke can run nested function containers.

Early delivery history lives under [history/](history/index.md) (history only, not a live roadmap).

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

  Client --> HostPort --> API
  API -->|"TCP TLS NOCTAXRIS_DOCKER_HOST"| Engine
  Engine --> FnNet
  Engine --> EcsNet
  Engine --> DataNet
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
  ├─ GET /_noctaxris/health → 200 ok (liveness)
  ├─ GET /_noctaxris/ready → 200 ready / 503 (SQLite ping; engine TLS dial when DockerHost set)
  ├─ GET /_noctaxris/version → 200 product semver (plain text)
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

Object bytes live under `$DATAROOT/s3/{account}/{bucket}/...`. Lambda zip contents live under `$DATAROOT/lambda/...` and are shared with DinD through the Compose `noctaxris-compute` volume (API-only `noctaxris-data` holds `master.key` / `state.db` and is not mounted on the engine; the engine mounts compute `:ro`). Bucket metadata, object metadata (etag, SSE), DynamoDB tables/items, SQS queues/messages, and Lambda function metadata live in SQLite. Cognito signing keys are sealed under the store master key. BCM export samples land under `$DATAROOT/bcm-exports/...`.

## Compute path

Compose sets `NOCTAXRIS_DOCKER_HOST=tcp://noctaxris-engine:2376` and `NOCTAXRIS_DOCKER_CERT_PATH=/certs/client` for TLS to the nested engine. The API process never mounts host `/var/run/docker.sock`. Runtime allowlists the Compose engine URL (extend with `NOCTAXRIS_DOCKER_HOST_ALLOWLIST`) and requires client TLS PEMs whenever Docker host is set. Default Compose runs `noctaxris-engine` as restricted DinD (`privileged: false`, explicit caps/devices, `cgroup: host`, writable `/sys/fs/cgroup`, dockerd `--ipv6=false`); see [security-defaults.md](security-defaults.md). Hosts that cannot start nested containers may opt in with `compose.engine-privileged.yaml`. The engine API is not published to the host. Function containers attach to DinD network `noctaxris-fn` with IP masquerade disabled (WAN deny; host-gateway reachability for the published lab API). Empty `NOCTAXRIS_DOCKER_HOST` disables compute so unit tests can run without DinD. Image pulls are limited to the lab registry and pinned lab bases (`NOCTAXRIS_IMAGE_PULL_ALLOWLIST` for extras).

Lambda, ECS, CodeBuild, Batch, and nested data engines all use nested DinD (`NOCTAXRIS_COMPUTE_RUNTIME` unset or `dind`). Unknown runtime values fail process start. Live zip/Image Invoke and ECS RunTask require a healthy `noctaxris-engine`. The API never falls through to host Docker.

## Nested data planes

RDS, ElastiCache, DocumentDB, MQ (RabbitMQ), and OpenSearch engine processes (when started) are nested containers via the same `noctaxris-engine` TLS client used for Lambda. Labels such as `noctaxris.data=rds|elasticache|docdb|mq|opensearch` identify them. Host Compose still publishes only `127.0.0.1:4566`.

```mermaid
flowchart TD
  Create["CreateDBInstance / CreateCacheCluster / CreateDBCluster / CreateBroker / CreateDomain"]
  Store["store row<br/>nested-network endpoint, secret ARN, status"]
  Helper["data-plane helper<br/>compute.Client DinD TLS"]
  Start["start labeled nested container<br/>no host port publish"]
  Describe["Describe* returns nested-network hostname:port"]
  DataAPI["RDS Data API ExecuteStatement on :4566<br/>pgx nested DSN or nested psql; else unavailable"]

  Create --> Store --> Helper --> Start --> Describe
  Helper -.-> DataAPI
```

Athena queries Glue catalog metadata and lab S3 object bytes **in-process** on the API (no nested query engine required). Nested MQ and OpenSearch promote to `RUNNING` / `Active` only after a healthy nested container; without DinD or on start failure they fail closed (`CREATION_FAILED` / `CreateFailed` with `stub://`). OpenSearch CreateFailed may include a lab `FailureReason` when nested logs match mmap / memory-lock bootstrap failures (`vm.max_map_count`). Broker and search ports are never host-published.

When DinD is unset, create paths keep control-plane rows and nested start is a no-op. Live engine start requires `noctaxris-engine`. Do not mount the operator host filesystem into nested data containers.

## Networking vocabulary

Noctaxris does not emulate Amazon VPC, ENIs, or PrivateLink. Nested compute and data engines use **DinD Internal** Docker networks (`noctaxris-fn`, `noctaxris-ecs`, `noctaxris-data`). Those are not AWS VPC private connectivity. Transfer omits `EndpointType=VPC`. Cloud Map private DNS stores a lab-opaque `Vpc` string only. Lambda `VpcConfig` and ECS `awsvpcConfiguration` fail closed.

## In-process delivery workers

- EventBridge Scheduler advances due schedules inside the API process and delivers via existing Lambda async enqueue, SQS SendMessage, and SNS Publish helpers.
- Lambda SQS event source mappings poll continuously with ReceiveMessage, synchronously Invoke, and DeleteMessage on success.
- EventBridge Pipes expose poll helpers for SQS, DynamoDB Streams, and EventBridge bus sources, with optional Lambda enrichment. An in-process ticker calls `PollPipeOnce` for RUNNING pipes.
- SNS HTTP(S) subscriptions deliver only to the lab catcher or exact allowlisted public URLs, with a pinned dialer (no redirects; private/loopback/metadata hosts rejected on allowlist paths).

## Edge identity

- Cognito issues RS256 ID and access tokens and serves JWKS on the same `:4566` listener. Issuer shape: `http://127.0.0.1:4566/cognito-idp/<region>/<userPoolId>`.
- API Gateway HTTP API JWT authorizer verifies Bearer tokens via the shared jose helper against lab Cognito JWKS (in-process; no remote JWKS by default). IAM routes require SigV4 and `execute-api:Invoke` (no HTTP API resource policies).
- AppSync accepts `AMAZON_COGNITO_USER_POOLS` beside API_KEY and AWS_IAM. Custom issuers require `NOCTAXRIS_ALLOW_REMOTE_JWKS` and a public host allowlist.
- Gateway `CreateIntegration` optional `CredentialsArn` enforces PassRole plus `apigateway.amazonaws.com` trust. Without CredentialsArn, HTTP API Lambda invoke requires a function resource policy Allow for `apigateway.amazonaws.com`. AppSync Lambda data sources require the same for `appsync.amazonaws.com`.
- CloudFront remains a config-shaped stub (no real PoP). ELBv2 has an ALB lab listener path that invokes Lambda targets (no EC2/IP dataplane, no NLB). Gateway must not open HTTP_PROXY to arbitrary URLs.

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
- SSE-KMS uses lab KMS GenerateDataKey / Decrypt under the caller
- DynamoDB item ciphertext uses table SSE (AWS-owned or customer-managed KMS)
- SQS message bodies use SSE-SQS or SSE-KMS when queue attributes request encryption
- Lambda Invoke injects temporary AWS_* credentials for the function execution role
