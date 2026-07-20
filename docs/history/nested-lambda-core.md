# Nested Lambda core (historical)

Historical ship snapshot: the first lab-complete Lambda core with zip packaging for `python3.12`, CreateFunction through UpdateConfiguration, sync Invoke, PassRole plus `lambda.amazonaws.com` trust checks, nested DinD compute (no host `docker.sock`), execution-role session injection into the function container, and platform egress deny on the function network.

Current Lambda depth (versions and aliases, layers, async Invoke with SQS DLQ/OnFailure, Image packaging, multi-runtime zip, TLS engine on 2376, same-account resource policies) lives in [../services/lambda.md](../services/lambda.md).

## Delivered

| Area | Status |
|------|--------|
| CreateFunction, GetFunction, DeleteFunction, ListFunctions | Done |
| UpdateFunctionCode, UpdateFunctionConfiguration | Done |
| Sync Invoke (RequestResponse-style one-shot container) | Done |
| Zip upload (`Code.ZipFile`), runtime `python3.12` | Done at this ship. Later superseded: see [../services/lambda.md](../services/lambda.md) for Image, `python3.11` / `nodejs20.x`, layers, versions/aliases |
| PassRole + role trust for `lambda.amazonaws.com` on create/update role | Done |
| Nested engine via Compose `noctaxris-engine` (DinD, privileged) | Done at this ship. Later: TLS to the engine on port 2376 |
| No host `docker.sock` mount on the API container | Done |
| Execution-role temp credentials injected on Invoke | Done |
| Function network `noctaxris-fn` with `Internal: true` (egress deny) | Done |
| Current Lambda depth | [../services/lambda.md](../services/lambda.md) |

## Authz note

CreateFunction and UpdateFunctionConfiguration that set a role require the caller to Allow `iam:PassRole` on that role ARN, and the role trust policy must Allow `sts:AssumeRole` for service principal `lambda.amazonaws.com`. Root callers still pass the PassRole identity check through Evaluate, but trust must still name Lambda. Early Invoke used identity Allow for `lambda:InvokeFunction`. Current dataplane also Allows via same-account function resource policy (see [../services/lambda.md](../services/lambda.md)). Invoke still mints temporary credentials for the function role so in-function SDK calls hit the same emulator endpoint.

## Compute note

`internal/compute` talks to `NOCTAXRIS_DOCKER_HOST` (Compose default `tcp://noctaxris-engine:2376` with TLS via `NOCTAXRIS_DOCKER_CERT_PATH`). Function containers run inside DinD on an internal bridge. They do not get a default route to the public internet. That differs from AWS Lambda, where functions have internet egress by default unless you place them in a VPC without a NAT path. Reaching Noctaxris from inside a function still depends on `host.docker.internal` / host-gateway wiring (`NOCTAXRIS_LAMBDA_ENDPOINT_URL`). Current Lambda depth is in [../services/lambda.md](../services/lambda.md).

## Smoke (Compose)

See [../services/lambda.md](../services/lambda.md) for create-role with Lambda trust, zip and Image CreateFunction, layers, sync and async Invoke, versions/aliases, and resource policy smoke.

## Explicitly deferred at this ship

Current deferred depth and later-shipped Lambda surface: [../services/lambda.md](../services/lambda.md). At this ship there were no layers, event source mappings, async Invoke, Function URLs, container-image packaging, versions/aliases, function resource policies, or runtimes other than `python3.12` (those landed later and are documented on the service page).
