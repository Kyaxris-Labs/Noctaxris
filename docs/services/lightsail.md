# Lightsail

**Status:** shipped (lab core)

JSON 1.1 control-plane lab (`Lightsail_20161128.*`, signing name `lightsail`). Instances are a stored state machine only: no real VMs or nested compute.

## Implemented

| Area | Actions |
|------|---------|
| Catalog | `GetBlueprints`, `GetBundles` (static lab lists) |
| Instances | `CreateInstances`, `GetInstance`, `GetInstances`, `StartInstance`, `StopInstance`, `RebootInstance`, `DeleteInstance` |

### Authz notes

Identity `EvaluateFull` on `lightsail:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws lightsail get-blueprints --endpoint-url "$EP"
aws lightsail create-instances \
  --instance-names web-a \
  --availability-zone us-east-1a \
  --blueprint-id ubuntu_22_04 \
  --bundle-id nano_3_0 \
  --endpoint-url "$EP"
aws lightsail get-instance --instance-name web-a --endpoint-url "$EP"
aws lightsail stop-instance --instance-name web-a --endpoint-url "$EP"
aws lightsail delete-instance --instance-name web-a --endpoint-url "$EP"
```

Unit tests cover create/stop/start/delete state transitions.

## Not yet / deferred

- Disks, static IPs, key pairs, networking ports
- Container services, databases, load balancers, snapshots
- Real VM or nested-compute launch
