# IoT Core and IoT Data

**Status:** shipped (lab lite)

Control-plane Things / certificates / policies / principals / topic rules, plus HTTP JSON and REST thing shadows. Optional MQTT shadow + rules bridge when shared Mosquitto is enabled.

**Protocol:** JSON 1.1 lab facade plus REST data-plane paths on `:4566`  
**Targets:** `AWSIotService.<Action>` (control plane, SigV4 service `iot`) and `AWSIotDataService.<Action>` (shadows, SigV4 service `iot-data`). REST shadows use SigV4 service `iotdevicegateway`. Jobs data plane uses `iot-jobs-data`.

## MQTT wire (opt-in)

| Mode | Behavior |
|------|----------|
| `NOCTAXRIS_SHARED_MQTT` off (default) | Control plane + HTTP shadows + topic rules CRUD/dispatch via `PublishTopic` unchanged; no MQTT listener |
| `NOCTAXRIS_SHARED_MQTT=1` | Singleton `noctaxris-lab-mqtt` (Eclipse Mosquitto, allowlisted pin) on Internal `noctaxris-data`; API bridge dials `noctaxris-engine:1883` on the Compose network for shadows and non-`$aws/` publishes (rules) |

Nested MQTT clients (Lambda/ECS/other DinD containers) use `noctaxris-lab-mqtt:1883`. Operator loopback `127.0.0.1:1883` requires `docker/compose.lab-nested-ports.yaml` on the engine hop. Enable brokers with `docker/compose.lab-brokers.yaml` (sets shared Kafka/MQTT and `NOCTAXRIS_BROKER_PORT_PUBLISH=1` for narrow engine publish). Without shared MQTT or engine publish, bridge start fails closed (no anonymous listener, no silent no-op). Host `:1883` is not published by default.

### Device authentication (lab IoT CA)

Mosquitto requires client certificates signed by the process **lab IoT CA** (material under the lab secrets sibling path, not operator upload APIs). `CreateKeysAndCertificate` signs device certs with **ClientAuth** EKU and embeds `certificateId` for mapping. Mosquitto: `require_certificate true`, `allow_anonymous false`, ACL allowlist for `$aws/things/+/shadow/#` and application topics (`#`) at the broker layer (IoT policies still govern each action).

Certificates issued before lab-CA signing cannot authenticate to MQTT until re-issued. This is not Bring-Your-Own-CA or full CSR/operator CA depth.

### MQTT authorization

The bridge and broker path use `EvaluateIoTDevicePolicy` on the union of IoT policies attached to the certificate: actions `iot:Connect`, `iot:Publish`, `iot:Subscribe`, `iot:Receive` against topic / topicfilter / client ARN shapes. Deny-overrides; no matching Allow fails closed. Account binding comes from the ACTIVE certificate attached to a Thing (TLS certificate id), not from MQTT `clientId` alone.

MQTT **ClientId must equal the attached thing name** (`${iot:Connection.Thing.ThingName}`). Filename-shaped client IDs (for example `device.pem`) are denied. Live shadow handling:

| Thing certs | Behavior |
|-------------|----------|
| One ACTIVE attached cert | Used for policy evaluation |
| Multiple ACTIVE; exactly one Allows the shadow action | That cert is used |
| Multiple ACTIVE; zero or multiple Allow | Fail closed (`/rejected`); pass explicit TLS `certificateId` to lab tooling, or detach extra certs |

Control-plane and HTTP shadow APIs keep identity `EvaluateFull`.

Mosquitto TLS material is bind-mounted into `noctaxris-lab-mqtt`. If broker config/CA mounts change after the container first started, Ensure recreates the singleton automatically (binds fingerprint mismatch).

### Shadow topics (classic + named)

| Topic pattern | Store action |
|---------------|--------------|
| `$aws/things/{thing}/shadow/update` (+ `/name/{shadow}/update`) | `UpdateIoTThingShadow`; publishes accepted/rejected |
| `…/get` | Get shadow; `/get/accepted` |
| `…/delete` | Delete; accepted/rejected |

Non-`$aws/` MQTT publishes (when shared MQTT is on) enter the topic rules engine. Unit tests use in-process `PublishTopic` so Mosquitto is not required.

## Topic rules (lite)

| Action | Behavior |
|--------|----------|
| `CreateTopicRule` / `ReplaceTopicRule` | Persist `ruleName`, `sql`, `description`, `ruleDisabled`, `actions` JSON |
| `GetTopicRule` / `ListTopicRules` | Read stored rules |
| `DeleteTopicRule` | Remove rule |
| `EnableTopicRule` / `DisableTopicRule` | Toggle `ruleDisabled` |

SQL is minimal: `SELECT * FROM 'topic/filter'` (single quotes or double). Topic filters support MQTT `+` and `#` wildcards.

Supported action shapes (Floci-like; missing targets log-skip, never panic):

| Action key | Fields | Dispatch |
|------------|--------|----------|
| `republish` | `topic` | Re-publish without re-evaluating rules; MQTT republish when bridge is connected |
| `sqs` | `queueUrl` (optional `useBase64`) | `SendMessage` |
| `sns` | `topicArn` or `targetArn` | `Publish` |
| `s3` | `bucketName`/`bucket`, `key` | `PutObject` |
| `dynamoDB` / `dynamoDBv2.putItem` | `tableName` | `PutItem` from JSON object payload |
| `kinesis` | `streamName` (optional `partitionKey`) | `PutRecord` |
| `lambda` | `functionName` or `functionArn` | async `EnqueueAsyncInvoke` |

## Implemented

| Area | Actions |
|------|---------|
| Things | `CreateThing`, `DescribeThing`, `ListThings`, `UpdateThing`, `DeleteThing` |
| Certificates | `CreateKeysAndCertificate`, `DescribeCertificate`, `ListCertificates`, `UpdateCertificate`, `DeleteCertificate` (lab CA-signed PEM when MQTT path is in use) |
| Policies | `CreatePolicy`, `GetPolicy`, `ListPolicies`, `DeletePolicy`, `AttachPolicy`, `DetachPolicy` |
| Principals | `AttachThingPrincipal`, `ListThingPrincipals` |
| Topic rules | `CreateTopicRule`, `GetTopicRule`, `ListTopicRules`, `ReplaceTopicRule`, `DeleteTopicRule`, `EnableTopicRule`, `DisableTopicRule` + in-process action dispatch |
| Shadows | `UpdateThingShadow`, `GetThingShadow`, `DeleteThingShadow` (classic + optional `shadowName` / REST `?name=`) over HTTP JSON 1.1 and REST `GET|POST|DELETE /things/{thingName}/shadow`; same SQLite store over MQTT when shared MQTT is on |
| Named shadows | `ListNamedShadowsForThing` REST `GET /api/things/shadow/ListNamedShadowsForThing/{thingName}` (classic unnamed shadow omitted; empty list if only classic exists or the thing is unknown) |
| Endpoints | `DescribeEndpoint` JSON 1.1 and REST `GET /endpoint?endpointType=` for `iot:Data`, `iot:Data-ATS`, `iot:Jobs`, `iot:CredentialProvider` (lab `endpointAddress`, default `127.0.0.1:4566`) |
| Jobs | Control-plane `CreateJob` / `DescribeJob`; device HTTP `GET /things/{thingName}/jobs`, `GET /things/{thingName}/jobs/{jobId}`, `PUT /things/{thingName}/jobs/$next` (`iot-jobs-data:*`) |
| Credentials | mTLS `GET /role-aliases/{roleAlias}/credentials` with `x-amzn-iot-thingname` matching the certificate thing, device policy `iot:AssumeRoleWithCertificate`, IAM trust `credentials.iot.amazonaws.com`, TLS SNI matching the CredentialProvider `endpointAddress` |
| Retained MQTT | `ListRetainedMessages` returns an empty list (HTTP 200). Lab MQTT has no retained store |

### Notes

- Identical `CreateThing` is idempotent; conflicting attributes return `ResourceAlreadyExistsException`.
- Active or thing-attached certificates cannot be deleted (`DeleteConflictException`).
- Attached policies cannot be deleted until detached.
- Shadows merge `state` maps; null child keys delete. Version increments on each update.
- Topic rule SQL must include a quoted `FROM` topic filter; duplicate `ruleName` returns `ResourceAlreadyExistsException`.

### Authz notes

Identity `EvaluateFull` on `iot:*`, `iot-data:*`, and `iot-jobs-data:*` for signed HTTP APIs. Device certificates use attached IoT policies (`iot:*`). Device certificates cannot call `ListThings` or `ListRoleAliases`.

`GET /endpoint` is IoT only when SigV4 service is `iot` (or another `iot*` service). Path-style S3 `GET /endpoint` stays S3.

### MQTT ClientId

Connect policies that use `${iot:Connection.Thing.ThingName}` require the MQTT ClientId to equal the thing name. The TLS certificate still supplies identity. Using the certificate id or a filename as ClientId is denied.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

Lab protocol is JSON (`AWSIotService.*` / `AWSIotDataService.*` X-Amz-Target). Prefer SDK triples under `tests/sdk/`. Live Compose smoke skipped when Docker is unavailable.

MQTT shadow / rules round-trip needs DinD, `compose.lab-brokers.yaml`, and device certs from `CreateKeysAndCertificate` after shared MQTT is enabled. SDK live MQTT rows soft-skip when shared flags or engine are unavailable. Topic-rule unit tests use `PublishTopic` and do not require Mosquitto.

## Not yet / deferred

- Richer IoT SQL (WHERE, SELECT projections, nested functions)
- Thing types, thing groups, fleet indexing
- Operator custom CA upload APIs
- WAN ATS MQTT hostnames (nested / engine endpoints only)
- HTTP `iot-data:Publish` catalog action (in-process `PublishTopic` covers lab tests)
