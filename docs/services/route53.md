# Route 53

**Status:** shipped (lab core)

Hosted zone and A/CNAME resource record set lite, plus Alias A records to in-account CloudFront or ELB DNS names. Identity authz. Lab uses JSON control-plane shape on the shared listener.

## Implemented

| Area | Actions |
|------|---------|
| Zones | `CreateHostedZone`, `DeleteHostedZone`, `ListHostedZones` |
| Records | `ChangeResourceRecordSets`, `ListResourceRecordSets` (A and CNAME) |
| Alias | Type `A` `AliasTarget` when `DNSName` matches an in-account CloudFront `DomainName` or ELB `DNSName` (case-insensitive; trailing dot normalized). Unknown targets fail closed (`InvalidChangeBatch`). `EvaluateTargetHealth` is stored and returned but not evaluated. No recursive DNS and no HTTP fetch of the target. |
| Lab inject | `InjectQueryLogs` (`NoctaxrisRoute53.InjectQueryLogs`) when `NOCTAXRIS_ROUTE53_QUERY_LOG_INJECT=1` writes canned query-log lines to a CloudWatch Logs group/stream |

### Authz notes

Identity `EvaluateFull` on `route53:*`. Lab inject remains AccessDenied unless `NOCTAXRIS_ROUTE53_QUERY_LOG_INJECT=1`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws route53 create-hosted-zone --name lab.example.com --caller-reference "lab-1" --endpoint-url "$EP"
aws route53 change-resource-record-sets --hosted-zone-id "$ZID" --change-batch file://change.json --endpoint-url "$EP"
aws route53 list-resource-record-sets --hosted-zone-id "$ZID" --endpoint-url "$EP"
```

Example Alias change batch (substitute a Deployed CloudFront `DomainName` or ELB `DNSName`):

```json
{
  "Changes": [{
    "Action": "CREATE",
    "ResourceRecordSet": {
      "Name": "www.lab.example.com",
      "Type": "A",
      "AliasTarget": {
        "HostedZoneId": "Z2FDTNDATAQYW2",
        "DNSName": "dEXAMPLE.cloudfront.noctaxris.local",
        "EvaluateTargetHealth": false
      }
    }
  }]
}
```

Query log inject needs `NOCTAXRIS_ROUTE53_QUERY_LOG_INJECT=1` on the API process, then signed `InjectQueryLogs` (SigV4 JSON) to an existing log group. Local resolver stub is not included. Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Alias Type AAAA; health-check evaluation and traffic policies
- Resolver endpoints; live recursive DNS query capture (inject only)
- Full REST/XML Route 53 protocol parity
