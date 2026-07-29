# Lightsail

**Status:** shipped (lab core)

JSON 1.1 control-plane lab (`Lightsail_20161128.*`, signing name `lightsail`). Instances, disks, static IPs, key pairs, and public ports are stored-state only: no real VMs or nested compute. Key pair create returns dummy base64 material (not usable for SSH).

## Implemented

| Area | Actions |
|------|---------|
| Catalog | `GetBlueprints`, `GetBundles` (static lab lists) |
| Instances | `CreateInstances`, `GetInstance`, `GetInstances`, `StartInstance`, `StopInstance`, `RebootInstance`, `DeleteInstance` |
| Disks | `CreateDisk`, `GetDisk`, `GetDisks`, `AttachDisk`, `DetachDisk`, `DeleteDisk` |
| Static IPs | `AllocateStaticIp`, `GetStaticIp`, `GetStaticIps`, `AttachStaticIp`, `DetachStaticIp`, `ReleaseStaticIp` |
| Key pairs | `CreateKeyPair`, `GetKeyPair`, `GetKeyPairs`, `DeleteKeyPair` |
| Ports | `OpenInstancePublicPorts`, `CloseInstancePublicPorts`, `GetInstancePortStates` |

### Authz notes

Identity `EvaluateFull` on `lightsail:*`.

### Behavior notes

- Disks require `sizeInGb >= 8`; attach fails if already attached; delete fails while attached.
- Attach static IP updates the instance `publicIpAddress` and `isStaticIp`; detach/release restores an ephemeral lab address.
- Delete instance detaches disks/static IPs and drops port rows.
- Port open/close is metadata only (not enforced on any dataplane).

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
aws lightsail create-disk \
  --disk-name data-1 \
  --availability-zone us-east-1a \
  --size-in-gb 32 \
  --endpoint-url "$EP"
aws lightsail attach-disk \
  --disk-name data-1 \
  --instance-name web-a \
  --disk-path /dev/xvdf \
  --endpoint-url "$EP"
aws lightsail allocate-static-ip --static-ip-name web-ip --endpoint-url "$EP"
aws lightsail attach-static-ip \
  --static-ip-name web-ip \
  --instance-name web-a \
  --endpoint-url "$EP"
aws lightsail create-key-pair --key-pair-name lab-key --endpoint-url "$EP"
aws lightsail open-instance-public-ports \
  --instance-name web-a \
  --port-info fromPort=80,toPort=80,protocol=tcp \
  --endpoint-url "$EP"
aws lightsail get-instance-port-states --instance-name web-a --endpoint-url "$EP"
aws lightsail delete-instance --instance-name web-a --endpoint-url "$EP"
```

Unit tests cover instance lifecycle plus disk/static IP/key pair/port store and handler paths.

## Not yet / deferred

- Container services, databases, load balancers, snapshots
- Real VM or nested-compute launch
- ImportKeyPair / DownloadDefaultKeyPair
