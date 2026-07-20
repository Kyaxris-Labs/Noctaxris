# Route 53

**Status:** shipped (lab core)

Hosted zone and A/CNAME resource record set lite. Identity authz. Lab uses JSON control-plane shape on the shared listener.

## Implemented

| Area | Actions |
|------|---------|
| Zones | `CreateHostedZone`, `DeleteHostedZone`, `ListHostedZones` |
| Records | `ChangeResourceRecordSets`, `ListResourceRecordSets` (A and CNAME) |

### Authz notes

Identity `EvaluateFull` on `route53:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws route53 create-hosted-zone --name lab.example.com --caller-reference "lab-1" --endpoint-url "$EP"
aws route53 change-resource-record-sets --hosted-zone-id "$ZID" --change-batch file://change.json --endpoint-url "$EP"
aws route53 list-resource-record-sets --hosted-zone-id "$ZID" --endpoint-url "$EP"
```

Local resolver stub is not included. Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Alias targets to CloudFront/ELB
- Traffic policies, health checks, Resolver endpoints
- Full REST/XML Route 53 protocol parity
