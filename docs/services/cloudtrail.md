# CloudTrail

**Status:** shipped (lab core)

Lab LookupEvents over the existing JSONL audit under the data root `cloudtrail/` directory, plus CreateTrail with StartLogging delivery to in-account S3 and/or CloudWatch Logs. Identity authz. PassRole when `CloudWatchLogsRoleArn` is set (`cloudtrail.amazonaws.com` trust).

## Implemented

| Area | Actions |
|------|---------|
| Lookup | `LookupEvents` |
| Trails | `CreateTrail`, `DescribeTrails`, `DeleteTrail`, `StartLogging`, `StopLogging` |

Filters for LookupEvents: `StartTime` / `EndTime`, one `LookupAttributes` entry (`EventName`, `Username`, `EventId`, `EventSource`, `AccessKeyId`, `ReadOnly`), and `MaxResults` (1-50, default 50). Events return newest first. Each `Events[]` item includes `CloudTrailEvent` as the raw JSON line string.

### Trail delivery lite

`CreateTrail` requires an in-account S3 bucket (`S3BucketName`, optional `S3KeyPrefix`). Optional `CloudWatchLogsLogGroupArn` + `CloudWatchLogsRoleArn` (PassRole + trust). Trails start with `IsLogging=false`.

`StartLogging` delivers a lab snapshot of recent JSONL lines (capped) once to configured destinations, then sets `IsLogging=true`:

- S3 key `{prefix}AWSLogs/{account}/CloudTrail/noctaxris-{trail}-{ts}.json`
- and/or Logs stream `noctaxris-trail-{name}` via `PutLogEvents`

If a configured destination Put fails, StartLogging fails and `IsLogging` stays false. `StopLogging` clears the flag only (no further delivery). LookupEvents remains JSONL-based (StartLogging delivery is additive, not a continuous trail shipper).

### Authz notes

Identity `EvaluateFull` on `cloudtrail:*` with resource `*`. Org SCP/RCP filters apply. No trail resource policy path.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws s3api create-bucket --bucket ct-lab --endpoint-url "$EP"
aws logs create-log-group --log-group-name /noctaxris/cloudtrail --endpoint-url "$EP"

aws cloudtrail create-trail \
  --name lab-trail \
  --s3-bucket-name ct-lab \
  --cloud-watch-logs-log-group-arn "arn:aws:logs:us-east-1:000000000001:log-group:/noctaxris/cloudtrail" \
  --cloud-watch-logs-role-arn "arn:aws:iam::000000000001:role/ct-logs" \
  --endpoint-url "$EP"

aws sts get-caller-identity --endpoint-url "$EP"
aws cloudtrail start-logging --name lab-trail --endpoint-url "$EP"

aws s3api list-objects-v2 --bucket ct-lab --endpoint-url "$EP"
aws cloudtrail lookup-events \
  --lookup-attributes AttributeKey=EventName,AttributeValue=GetCallerIdentity \
  --max-results 10 \
  --endpoint-url "$EP"
aws cloudtrail stop-logging --name lab-trail --endpoint-url "$EP"
```

PassRole for the Logs role requires `cloudtrail.amazonaws.com` trust and caller `iam:PassRole`. Skip Logs branch when only testing S3 delivery.

## Not yet / deferred

- Continuous delivery after StartLogging (lab ships one snapshot per StartLogging call)
- PutEventSelectors / Insights / Lake / organization trails
- Multi-attribute LookupAttributes (AWS allows one today in this lab as well)
- Event history retention policies beyond the lab JSONL file
- Cross-account LookupEvents
