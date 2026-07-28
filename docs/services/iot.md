# IoT Core and IoT Data

**Status:** shipped (lab lite)

Control-plane Things / certificates / policies / principals, plus HTTP JSON thing shadows. Optional MQTT shadow bridge when shared Mosquitto is enabled.

**Protocol:** JSON 1.1 lab facade  
**Targets:** `AWSIotService.<Action>` (control plane, SigV4 service `iot`) and `AWSIotDataService.<Action>` (shadows, SigV4 service `iot-data`)

## MQTT wire (opt-in)

| Mode | Behavior |
|------|----------|
| `NOCTAXRIS_SHARED_MQTT` off (default) | Control plane + HTTP shadows unchanged; no MQTT listener |
| `NOCTAXRIS_SHARED_MQTT=1` | Singleton `noctaxris-lab-mqtt` (Eclipse Mosquitto, allowlisted pin) on Internal `noctaxris-data`; API shadow bridge dials `noctaxris-engine:1883` on the Compose network |

Nested MQTT clients (Lambda/ECS/other DinD containers) use `noctaxris-lab-mqtt:1883`. Operator loopback `127.0.0.1:1883` requires `docker/compose.lab-nested-ports.yaml` on the engine hop. Enable brokers with `docker/compose.lab-brokers.yaml` (sets shared Kafka/MQTT and `NOCTAXRIS_BROKER_PORT_PUBLISH=1` for narrow engine publish). Without shared MQTT or engine publish, bridge start fails closed (no anonymous listener, no silent no-op).

### Device authentication (lab IoT CA)

Mosquitto requires client certificates signed by the process **lab IoT CA** (material under the lab secrets sibling path, not operator upload APIs). `CreateKeysAndCertificate` signs device certs with **ClientAuth** EKU and embeds `certificateId` for mapping. Mosquitto: `require_certificate true`, `allow_anonymous false`, ACL allowlist for `$aws/things/+/shadow/#` only at the broker layer (IoT policies still govern each action).

Certificates issued before lab-CA signing cannot authenticate to MQTT until re-issued. This is not Bring-Your-Own-CA or full CSR/operator CA depth.

### MQTT authorization

The bridge and broker path use `EvaluateIoTDevicePolicy` on the union of IoT policies attached to the certificate: actions `iot:Connect`, `iot:Publish`, `iot:Subscribe`, `iot:Receive` against topic / topicfilter / client ARN shapes. Deny-overrides; no matching Allow fails closed. Account binding comes from the ACTIVE certificate attached to a Thing, not from MQTT `clientId` alone.

Prefer setting the MQTT **ClientId to the device `certificateId`** (same hex as DER SHA-256) and pass that id into lab tooling when available. Live shadow handling:

| Thing certs | Behavior |
|-------------|----------|
| One ACTIVE attached cert | Used for policy evaluation |
| Multiple ACTIVE; exactly one Allows the shadow action | That cert is used |
| Multiple ACTIVE; zero or multiple Allow | Fail closed (`/rejected`); pass explicit `certificateId` / ClientId, or detach extra certs |

Control-plane and HTTP shadow APIs keep identity `EvaluateFull`.

Mosquitto TLS material is bind-mounted into `noctaxris-lab-mqtt`. If broker config/CA mounts change after the container first started, Ensure recreates the singleton automatically (binds fingerprint mismatch).

### Shadow topics (classic + named)

| Topic pattern | Store action |
|---------------|--------------|
| `$aws/things/{thing}/shadow/update` (+ `/name/{shadow}/update`) | `UpdateIoTThingShadow`; publishes accepted/rejected |
| `…/get` | Get shadow; `/get/accepted` |
| `…/delete` | Delete; accepted/rejected |

Other topics are broker-only when policy Allows; there is no Rules Engine dispatch yet.

## Implemented

| Area | Actions |
|------|---------|
| Things | `CreateThing`, `DescribeThing`, `ListThings`, `UpdateThing`, `DeleteThing` |
| Certificates | `CreateKeysAndCertificate`, `DescribeCertificate`, `ListCertificates`, `UpdateCertificate`, `DeleteCertificate` (lab CA-signed PEM when MQTT path is in use) |
| Policies | `CreatePolicy`, `GetPolicy`, `ListPolicies`, `DeletePolicy`, `AttachPolicy`, `DetachPolicy` |
| Principals | `AttachThingPrincipal`, `ListThingPrincipals` |
| Shadows | `UpdateThingShadow`, `GetThingShadow`, `DeleteThingShadow` (classic + optional `shadowName`) over HTTP; same SQLite store over MQTT when shared MQTT is on |

### Notes

- Identical `CreateThing` is idempotent; conflicting attributes return `ResourceAlreadyExistsException`.
- Active or thing-attached certificates cannot be deleted (`DeleteConflictException`).
- Attached policies cannot be deleted until detached.
- Shadows merge `state` maps; null child keys delete. Version increments on each update.

### Authz notes

Identity `EvaluateFull` on `iot:*` and `iot-data:*` for HTTP control/data APIs.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

Lab protocol is JSON (`AWSIotService.*` / `AWSIotDataService.*` X-Amz-Target). Prefer SDK triples under `tests/sdk/`. Live Compose smoke skipped when Docker is unavailable.

MQTT shadow round-trip needs DinD, `compose.lab-brokers.yaml`, and device certs from `CreateKeysAndCertificate` after shared MQTT is enabled. SDK live MQTT rows soft-skip when shared flags or engine are unavailable.

## Not yet / deferred

- IoT Rules engine depth (`CreateTopicRule` + action dispatch)
- Jobs control/data plane
- Thing types, thing groups, fleet indexing
- Operator custom CA upload APIs; retained-message management APIs
- WAN ATS MQTT hostnames (nested / engine endpoints only)
