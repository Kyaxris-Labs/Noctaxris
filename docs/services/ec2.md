# EC2

**Status:** shipped (lab nested instances + VPC Flow)

Query-protocol instance control plane (`Action=RunInstances`, credential scope `ec2`) plus JSON VPC Flow Logs under the same service. Instances are **nested Docker containers** on Internal network `noctaxris-ec2` inside Compose `noctaxris-engine` (TLS DinD). Not host VMs. No host `docker.sock`.

## Instance execution model

| EC2 state | Nested Docker |
|-----------|---------------|
| `pending` → `running` | Container create + start (`tail -f /dev/null` keep-alive) |
| `running` → `stopping` → `stopped` | `docker stop` (container kept) |
| `stopped` → `pending` → `running` | `docker start` (or create if never started) |
| → `shutting-down` → `terminated` | `docker rm -f` |

Without `NOCTAXRIS_DOCKER_HOST` / engine, `RunInstances` still returns instance IDs in **`pending`** (honest; no synthetic `running`). Soft-skip SDK checks wait for `running`.

### AMI → Docker map

| AMI ID / alias | Docker image (allowlisted) |
|----------------|----------------------------|
| `ami-alpine` | `public.ecr.aws/docker/library/alpine:3.20` |
| `ami-amazonlinux2023` | `public.ecr.aws/amazonlinux/amazonlinux:2023` |
| `ami-ubuntu2204` | `public.ecr.aws/docker/library/ubuntu:22.04` |
| unknown AMI | default alpine pin above |

## Implemented

| Area | Actions |
|------|---------|
| Instances | `RunInstances`, `DescribeInstances`, `TerminateInstances`, `StopInstances`, `StartInstances` |
| Images | `DescribeImages` (lab catalog; optional `ImageId.N` filter) |
| VPC Flow | See [vpcflow.md](vpcflow.md): `CreateFlowLogs` lite, lab `InjectFlowLogs` (opt-in) |

### Authz notes

Identity `EvaluateFull` on `ec2:RunInstances` / `DescribeInstances` / `DescribeImages` / `StopInstances` / `StartInstances` / `TerminateInstances`. Image pulls use the shared allowlist (`internal/compute/image_allow.go`); extend with `NOCTAXRIS_IMAGE_PULL_ALLOWLIST` when needed.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Nested running state needs `noctaxris-engine`.

```bash
aws ec2 describe-images --image-ids ami-alpine --endpoint-url "$EP"
aws ec2 run-instances \
  --image-id ami-alpine \
  --instance-type t3.micro \
  --count 1 \
  --endpoint-url "$EP"
IID=$(aws ec2 describe-instances --endpoint-url "$EP" \
  --query 'Reservations[0].Instances[0].InstanceId' --output text)
aws ec2 stop-instances --instance-ids "$IID" --endpoint-url "$EP"
aws ec2 start-instances --instance-ids "$IID" --endpoint-url "$EP"
aws ec2 terminate-instances --instance-ids "$IID" --endpoint-url "$EP"
```

Unit tests cover pending-without-engine, DescribeImages catalog/filter, unknown-AMI alpine pin, stop/start/terminate state changes, and CreateFlowLogs still routing after RunInstances. SDK Run/Stop/Start/Terminate soft-skips when the API is down. Terraform omits `aws_instance` (provider waits for `running`; pending without DinD hangs); ASG path is `STACK=lab-parity-compute` (`TF_PARITY=1`).

## Not yet / deferred

- Security groups, ENIs, VPC/subnet plane, SSH key injection, UserData execution, IMDS on instances
- RebootInstances, host port publish / socat forwards
- Owner/Filter name depth beyond ImageId for DescribeImages
- ASG DesiredCapacity → RunInstances reconcile (see [autoscaling.md](autoscaling.md))
