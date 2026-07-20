# Cloud Map (Service Discovery)

**Status:** shipped (lab core)

Private DNS or HTTP namespace, service, instance register/deregister, and DiscoverInstances lite.

## Implemented

| Area | Actions |
|------|---------|
| Namespaces | `CreatePrivateDnsNamespace`, `CreateHttpNamespace` |
| Services | `CreateService` |
| Instances | `RegisterInstance`, `DeregisterInstance`, `DiscoverInstances` |

### Authz notes

Identity `EvaluateFull` on `servicediscovery:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws servicediscovery create-private-dns-namespace --name lab.local --endpoint-url "$EP"
aws servicediscovery create-service --name api --namespace-id "$NS" --endpoint-url "$EP"
aws servicediscovery register-instance --service-id "$SVC" --instance-id i-1 \
  --attributes AWS_INSTANCE_IPV4=10.0.0.5 --endpoint-url "$EP"
aws servicediscovery discover-instances --namespace-name lab.local --service-name api --endpoint-url "$EP"
```

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Full DNS integration with Route 53 beyond simple linkage
- Health checks depth, public DNS namespaces
