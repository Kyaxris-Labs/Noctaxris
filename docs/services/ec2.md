# EC2

**Status:** shipped (lab nested instances + UserData/IMDS lite + VPC/SG/ENI metadata + VPC Flow)

Query-protocol instance control plane (`Action=RunInstances`, credential scope `ec2`) plus VPC/subnet/security-group/ENI **metadata** CRUD and JSON VPC Flow Logs under the same service. Instances are **nested Docker containers** on Internal network `noctaxris-ec2` inside Compose `noctaxris-engine` (TLS DinD). Not host VMs. No host `docker.sock`. Security group rules are persisted only; they are not enforced on the nested Docker network, and no host port publish / socat is used.

## Instance execution model

| EC2 state | Nested Docker |
|-----------|---------------|
| `pending` → `running` | Container create + start (`tail -f /dev/null` keep-alive); UserData once; IMDS lite register |
| `running` → `stopping` → `stopped` | `docker stop` (container kept) |
| `stopped` → `pending` → `running` | `docker start` (or create if never started); UserData is **not** re-run on restart |
| → `shutting-down` → `terminated` | `docker rm -f` |

Without `NOCTAXRIS_DOCKER_HOST` / engine, `RunInstances` still returns instance IDs in **`pending`** (honest; no synthetic `running`). Soft-skip SDK checks wait for `running`.

### AMI → Docker map

| AMI ID / alias | Docker image (allowlisted) |
|----------------|----------------------------|
| `ami-alpine` | `public.ecr.aws/docker/library/alpine:3.20` |
| `ami-amazonlinux2023` | `public.ecr.aws/amazonlinux/amazonlinux:2023` |
| `ami-ubuntu2204` | `public.ecr.aws/docker/library/ubuntu:22.04` |
| unknown AMI | default alpine pin above |

### UserData

On first transition to `running` (container create), Noctaxris decodes base64 `UserData` when needed and runs the script once via nested `docker exec` (`sh`). Failures are logged; **`RunInstances` still succeeds** and the instance stays `running`. `DescribeInstances` XML omits the UserData body (store retains it for bootstrap). UserData is not re-executed on `StartInstances` after stop.

### Instance metadata (IMDS lite)

A DinD-only sidecar (`noctaxris-ec2-imds`) on Internal `noctaxris-ec2` serves classic paths under `AWS_EC2_METADATA_SERVICE_ENDPOINT` (`http://169.254.169.254:9255`). ExtraHosts maps `169.254.169.254` to the sidecar. **No host port publish** (not Floci host `:9169`). Lookup is by the requesting container IP.

| Path | Behavior |
|------|----------|
| `/latest/meta-data/instance-id` | Instance id |
| `/latest/meta-data/local-ipv4` | Nested `noctaxris-ec2` address (also written to store for ENI/NLB) |
| `/latest/meta-data/ami-id` | Launch ImageId |
| `/latest/meta-data/iam/security-credentials/` | Lab role name when `IamInstanceProfile` / `.Name` / `.Arn` was set at RunInstances (persisted for create-on-StartInstances) |

## Implemented

| Area | Actions |
|------|---------|
| Instances | `RunInstances`, `DescribeInstances`, `TerminateInstances`, `StopInstances`, `StartInstances` |
| Images | `DescribeImages` (lab catalog; optional `ImageId.N` filter) |
| Bootstrap | UserData exec once on create; IMDS lite sidecar (DinD-internal) |
| VPC / subnet | `CreateVpc`, `DeleteVpc`, `DescribeVpcs`; `CreateSubnet`, `DeleteSubnet`, `DescribeSubnets` (metadata: Ids, CidrBlock, State=`available`, AZ) |
| Security groups | `CreateSecurityGroup`, `DeleteSecurityGroup`, `DescribeSecurityGroups`, `AuthorizeSecurityGroupIngress` / `Egress`, `RevokeSecurityGroupIngress` / `Egress` (rules persisted; not enforced on DinD) |
| ENI | `DescribeNetworkInterfaces` (stored stubs + synthetic `eni-*` for instances with private IPs); `CreateNetworkInterface` stub for Terraform-shaped IDs |
| VPC Flow | See [vpcflow.md](vpcflow.md): `CreateFlowLogs` lite, lab `InjectFlowLogs` (opt-in) |

### Authz notes

Identity `EvaluateFull` on `ec2:RunInstances` / `DescribeInstances` / `DescribeImages` / `StopInstances` / `StartInstances` / `TerminateInstances` and the VPC / subnet / security-group / ENI actions listed above. Image pulls use the shared allowlist (`internal/compute/image_allow.go`); extend with `NOCTAXRIS_IMAGE_PULL_ALLOWLIST` when needed.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Nested running state needs `noctaxris-engine`.

```bash
aws ec2 describe-images --image-ids ami-alpine --endpoint-url "$EP"
UD=$(echo '#!/bin/sh
touch /tmp/noctaxris-ud
' | base64 -w0)
aws ec2 run-instances \
  --image-id ami-alpine \
  --instance-type t3.micro \
  --count 1 \
  --user-data "$UD" \
  --iam-instance-profile Name=LabEC2Role \
  --endpoint-url "$EP"
IID=$(aws ec2 describe-instances --endpoint-url "$EP" \
  --query 'Reservations[0].Instances[0].InstanceId' --output text)
# From inside the nested instance (engine exec): curl IMDS via AWS_EC2_METADATA_SERVICE_ENDPOINT
aws ec2 stop-instances --instance-ids "$IID" --endpoint-url "$EP"
aws ec2 start-instances --instance-ids "$IID" --endpoint-url "$EP"
aws ec2 terminate-instances --instance-ids "$IID" --endpoint-url "$EP"

VPC=$(aws ec2 create-vpc --cidr-block 10.0.0.0/16 --endpoint-url "$EP" \
  --query 'Vpc.VpcId' --output text)
aws ec2 create-subnet --vpc-id "$VPC" --cidr-block 10.0.1.0/24 \
  --availability-zone us-east-1a --endpoint-url "$EP"
SG=$(aws ec2 create-security-group --group-name lab-web --description 'lab web' \
  --vpc-id "$VPC" --endpoint-url "$EP" --query 'GroupId' --output text)
aws ec2 authorize-security-group-ingress --group-id "$SG" --protocol tcp \
  --port 80 --cidr 0.0.0.0/0 --endpoint-url "$EP"
aws ec2 describe-security-groups --group-ids "$SG" --endpoint-url "$EP"
aws ec2 describe-network-interfaces --endpoint-url "$EP"
```

Unit tests cover UserData base64 decode, IMDS endpoint/ExtraHosts (no host publish), pending-without-engine, DescribeImages catalog/filter, unknown-AMI alpine pin, stop/start/terminate state changes, private IP store updates, VPC/subnet/SG rule persistence, synthetic ENIs, CreateNetworkInterface stubs, and CreateFlowLogs still routing after RunInstances. Live DinD UserData/IMDS smoke soft-skips without `NOCTAXRIS_DOCKER_HOST`. SDK Run/Stop/Start/Terminate soft-skips when the API is down. Terraform omits `aws_instance` (provider waits for `running`; pending without DinD hangs); ASG path is `STACK=lab-parity-compute` (`TF_PARITY=1`).

## Not yet / deferred

- SSH key injection
- RebootInstances, host port publish / socat forwards
- Security group or ENI enforcement on nested Docker networking
- Owner/Filter name depth beyond ImageId for DescribeImages
- ASG DesiredCapacity → RunInstances reconcile (see [autoscaling.md](autoscaling.md))
