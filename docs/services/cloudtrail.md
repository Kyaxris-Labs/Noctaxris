# CloudTrail

**Status:** shipped (lab core)

Lab LookupEvents over the existing JSONL audit under the data root `cloudtrail/` directory. No separate trail shipper. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Lookup | `LookupEvents` |

Filters: `StartTime` / `EndTime`, one `LookupAttributes` entry (`EventName`, `Username`, `EventId`, `EventSource`, `AccessKeyId`, `ReadOnly`), and `MaxResults` (1-50, default 50). Events return newest first. Each `Events[]` item includes `CloudTrailEvent` as the raw JSON line string.

### Authz notes

Identity `EvaluateFull` on `cloudtrail:LookupEvents` with resource `*`. Org SCP/RCP filters apply. No trail resource policy path.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

Generate traffic, then look up:

```bash
aws sts get-caller-identity --endpoint-url "$EP"

aws cloudtrail lookup-events \
  --lookup-attributes AttributeKey=EventName,AttributeValue=GetCallerIdentity \
  --max-results 10 \
  --endpoint-url "$EP"
```

## Not yet / deferred

- CreateTrail / StartLogging / PutEventSelectors / Insights / Lake / organization trails
- Multi-attribute LookupAttributes (AWS allows one today in this lab as well)
- Event history retention policies and delivery to S3 or CloudWatch Logs
- Cross-account LookupEvents
