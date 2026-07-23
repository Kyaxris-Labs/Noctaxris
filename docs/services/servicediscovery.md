# Cloud Map (Service Discovery)

**Status:** shipped (lab core)

Private DNS or HTTP namespace, service, instance register/deregister, and DiscoverInstances lite. Private DNS requires a lab-opaque `Vpc` string (not EC2-validated). DiscoverInstances for private DNS requires the matching `Vpc`. DinD Internal networks are not Amazon VPC or PrivateLink.

## Implemented

| Area | Actions |
|------|---------|
| Namespaces | `CreatePrivateDnsNamespace` (requires `Vpc`), `CreateHttpNamespace` |
| Services | `CreateService` |
| Instances | `RegisterInstance`, `DeregisterInstance`, `DiscoverInstances` (private DNS scoped by `Vpc`) |

### Authz notes

Identity `EvaluateFull` on `servicediscovery:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws servicediscovery create-private-dns-namespace --name lab.local --vpc vpc-lab1 --endpoint-url "$EP"
aws servicediscovery create-service --name api --namespace-id "$NS" --endpoint-url "$EP"
aws servicediscovery register-instance --service-id "$SVC" --instance-id i-1 \
  --attributes AWS_INSTANCE_IPV4=10.0.0.5 --endpoint-url "$EP"
```

Pass `Vpc` on CreatePrivateDnsNamespace. DiscoverInstances requires matching `Vpc` (or `VpcId`) in the JSON body for private DNS namespaces (use `--cli-input-json` when the CLI lacks a Vpc flag). Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Full DNS integration with Route 53 / private hosted zones
- EC2-validated VPC IDs, health checks depth, public DNS namespaces
