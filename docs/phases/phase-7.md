# Phase 7 status

Phase 7 delivers a lab-complete Lambda core: zip packaging for `python3.12`, CreateFunction through UpdateConfiguration, sync Invoke, PassRole plus `lambda.amazonaws.com` trust checks, nested DinD compute (no host `docker.sock`), execution-role session injection into the function container, and platform egress deny on the function network.

## Delivered

| Area | Status |
|------|--------|
| CreateFunction, GetFunction, DeleteFunction, ListFunctions | Done |
| UpdateFunctionCode, UpdateFunctionConfiguration | Done |
| Sync Invoke (RequestResponse-style one-shot container) | Done |
| Zip upload only (`Code.ZipFile`), runtime `python3.12` | Done |
| PassRole + role trust for `lambda.amazonaws.com` on create/update role | Done |
| Nested engine via Compose `noctaxris-engine` (DinD, privileged) | Done |
| No host `docker.sock` mount on the API container | Done |
| Execution-role temp credentials injected on Invoke | Done |
| Function network `noctaxris-fn` with `Internal: true` (egress deny) | Done |
| Deferred Lambda depth | [../services/lambda.md](../services/lambda.md) |

## Authz note

CreateFunction and UpdateFunctionConfiguration that set a role require the caller to Allow `iam:PassRole` on that role ARN, and the role trust policy must Allow `sts:AssumeRole` for service principal `lambda.amazonaws.com`. Root callers still pass the PassRole identity check through Evaluate, but trust must still name Lambda. Invoke uses identity Allow for `lambda:InvokeFunction` on the function ARN, then mints temporary credentials for the function role so in-function SDK calls hit the same emulator endpoint.

## Compute note

`internal/compute` talks to `NOCTAXRIS_DOCKER_HOST` (Compose default `tcp://noctaxris-engine:2375`). Function containers run inside DinD on an internal bridge. They do not get a default route to the public internet. That differs from AWS Lambda, where functions have internet egress by default unless you place them in a VPC without a NAT path. Reaching Noctaxris from inside a function still depends on `host.docker.internal` / host-gateway wiring (`NOCTAXRIS_LAMBDA_ENDPOINT_URL`).

## Smoke (Compose)

See [../services/lambda.md](../services/lambda.md) for create-role with Lambda trust, zip CreateFunction, and Invoke.

## Explicitly not in Phase 7

See [../services/lambda.md](../services/lambda.md). No layers, event source mappings, async invoke, Function URLs, container-image packaging, or runtimes other than `python3.12`.
