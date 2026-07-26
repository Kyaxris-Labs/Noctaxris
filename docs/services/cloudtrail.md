# CloudTrail

**Status:** shipped (lab core)

Lab LookupEvents over the existing JSONL audit under the data root `cloudtrail/` directory, plus CreateTrail with StartLogging delivery to in-account S3 and/or CloudWatch Logs. After StartLogging, new JSONL lines are shipped continuously to the same destinations. Identity authz. PassRole when `CloudWatchLogsRoleArn` is set (`cloudtrail.amazonaws.com` trust).

Optional lab inject (`cloudtrail:InjectEvents`) seeds forensic-shaped JSONL events when explicitly enabled. Lab `InjectInsightsEvents` seeds insight-shaped records for `LookupEvents` with `EventCategory=insight` (no Insights ML engine). Live audit lines include `userIdentity.userName`, richer safe `requestParameters`, and `resources[]` on key S3/Lambda/Secrets paths. X-Forwarded-For for audit `sourceIPAddress` is opt-in only.

## Implemented

| Area | Actions |
|------|---------|
| Lookup | `LookupEvents` (`EventCategory=insight` returns Insight records only; default excludes Insights) |
| Trails | `CreateTrail`, `DescribeTrails`, `DeleteTrail`, `StartLogging`, `StopLogging` |
| Selectors | `PutEventSelectors`, `GetEventSelectors` (management + optional S3 data) |
| Digests | Digest sidecar after S3 delivery; lab `ValidateLogs` |
| Lab inject | `InjectEvents`, `InjectInsightsEvents` (Noctaxris lab extensions; not AWS public APIs) |

Filters for LookupEvents: `StartTime` / `EndTime`, one `LookupAttributes` entry (`EventName`, `Username`, `EventId`, `EventSource`, `AccessKeyId`, `ReadOnly`, plus lab `SourceIPAddress`), `EventCategory` (`insight` for Insights lite), and `MaxResults` (1-50, default 50). Events return newest first. Each `Events[]` item includes `CloudTrailEvent` as the raw JSON line string.

### Live audit shape

Success and error audit lines write AWS-shaped fields into `$DATAROOT/cloudtrail/events.jsonl`:

- `userIdentity` includes `type`, `accountId`, `accessKeyId`, `arn`, and `userName` (root / IAM user / role session name)
- `resources[]` on CreateBucket / PutObject / CreateFunction / CreateSecret / DeleteSecret (`accountId`, `type`, `ARN`)
- `requestParameters` always includes `httpMethod` and `path`; also `xAmzTarget` or query `action` when present. Handlers may add safe scalars (for example S3 `bucketName`/`key`, Lambda `functionName`, Secrets `name`/`secretId`). Bodies, passwords, tokens, and secrets are not logged
- `sourceIPAddress` defaults to the TCP peer (`RemoteAddr`). Set `NOCTAXRIS_CLOUDTRAIL_TRUST_XFF=1` to use the first `X-Forwarded-For` hop for audit only. Authz `aws:SourceIp` always stays the TCP peer
- `eventCategory` / `managementEvent` default to Management / true on live audit lines
- AssumedRole / Federated callers may include `userIdentity.sessionContext.sessionIssuer` lite
- JSON error audit lines use a service-derived `eventSource` (not a fixed Organizations source)
- Sibling `kms:Decrypt` audit lines after successful SSE-KMS / Secrets / SSM SecureString / DynamoDB SSE-KMS reads
- Authorable inject fields also include `eventCategory`, `managementEvent`, and `sessionContext`

### Lab event inject

AWS CloudTrail has no public API to inject arbitrary event history. Noctaxris exposes a lab-only action for forensic seeding:

- Env: `NOCTAXRIS_CLOUDTRAIL_INJECT=1` (default off; requests return `AccessDeniedException`)
- Target: `NoctaxrisCloudTrail.InjectEvents` (SigV4 service `cloudtrail`)
- Authz: `cloudtrail:InjectEvents`
- Body: single `Event` object, top-level event fields, or `Events` array (cap 50)
- Authorable fields: `eventTime`, `sourceIPAddress`, `userIdentity` (including `userName`), `eventSource`, `eventName`, optional `resources`, `requestParameters`, `responseElements`, `errorCode` / `errorMessage`, `readOnly`, `awsRegion`, `userAgent`, `eventID` (generated when omitted)
- Before write, inject redacts password-like / secret keys in `requestParameters`, `responseElements`, and Insights `insightDetails` (for example `SecretString`, `SecretAccessKey`, `Password`, `*Token`, `credentials`) and applies nested size/depth caps. Prefer forensic metadata over secret material in inject payloads.

Injected lines append to the same `events.jsonl` file, so LookupEvents and trail StartLogging / continuous delivery behave unchanged.

### Lab Insights inject (no ML engine)

CloudTrail Insights in AWS is an anomaly detector. Noctaxris does not invent a baseline/ML engine. Labs seed insight-shaped JSONL records:

- Env: same `NOCTAXRIS_CLOUDTRAIL_INJECT=1` gate
- Target: `NoctaxrisCloudTrail.InjectInsightsEvents`
- Authz: `cloudtrail:InjectInsightsEvents`
- Body: `Event` / `Events` with required `insightDetails` (`eventName`, `eventSource`; optional `insightType` default `ApiCallRateInsight`, `state`, `insightContext`)
- Written records use `eventType=AwsCloudTrailInsight` and `eventCategory=Insight`
- Lookup: `LookupEvents` with `EventCategory=insight` (default LookupEvents excludes Insight lines)

```bash
# Enable inject in the API process env, then:
aws cloudtrail lookup-events --endpoint-url "$EP" # after seeding via signed InjectEvents
aws cloudtrail lookup-events --event-category insight --endpoint-url "$EP" # after InjectInsightsEvents
```

Example signed JSON body (lab SDK / curl + SigV4):

```json
{
  "Events": [
    {
      "eventTime": "2026-07-20T15:00:00Z",
      "sourceIPAddress": "198.51.100.10",
      "userIdentity": { "type": "IAMUser", "userName": "alice" },
      "eventSource": "signin.amazonaws.com",
      "eventName": "ConsoleLogin",
      "readOnly": false
    }
  ]
}
```

### Trail delivery lite

`CreateTrail` requires an in-account S3 bucket (`S3BucketName`, optional `S3KeyPrefix`). Optional `CloudWatchLogsLogGroupArn` + `CloudWatchLogsRoleArn` (PassRole + trust). Trails start with `IsLogging=false`.

`StartLogging` delivers a lab snapshot of recent JSONL lines (capped) once to configured destinations, sets `IsLogging=true`, and anchors a per-trail delivery cursor at the current JSONL line count:

- S3 key `{prefix}AWSLogs/{account}/CloudTrail/{region}/{yyyy}/{mm}/{dd}/{account}_CloudTrail_{region}_{yyyyMMddTHHmmZ}_{uniq}.json` (optional `.gz` when `NOCTAXRIS_CLOUDTRAIL_GZIP=1`)
- and/or Logs stream `noctaxris-trail-{name}` via `PutLogEvents` (each event timestamp prefers JSONL `eventTime`)

While `IsLogging=true`, each new audit JSONL append triggers continuous delivery of lines past the cursor to the same in-account S3 (and optional Logs) destinations. Selector filtering applies to S3/Logs delivery only (LookupEvents still reads the full account JSONL). Each ship writes a new S3 object (and optional Logs events) for the delta, then a digest sidecar under `CloudTrail-Digest/`, then advances the cursor. Delivery stays in-account only (loopback secure defaults).

`CreateTrail` accepts `IsOrganizationTrail` for the lab management account only (trail ARN uses `trail/{o-noctaxris}/{name}`). LookupEvents filters `recipientAccountId` to the caller account.

If a configured destination Put fails on StartLogging, the call fails and `IsLogging` stays false. If a continuous Put fails, that trail's `IsLogging` is cleared (fail closed), the cursor is not advanced, and the failure is logged; call StartLogging again after fixing the destination. `StopLogging` clears the flag only (no further continuous delivery). LookupEvents remains JSONL-based (S3/Logs delivery is additive).

### Authz notes

Identity `EvaluateFull` on `cloudtrail:*` with resource `*`. Org SCP/RCP filters apply. No trail resource policy path. Lab inject remains fail-closed unless `NOCTAXRIS_CLOUDTRAIL_INJECT=1`.

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

# Further API calls append audit JSONL and ship additional S3/Logs objects while logging.
aws sts get-caller-identity --endpoint-url "$EP"
aws s3api list-objects-v2 --bucket ct-lab --endpoint-url "$EP"

aws cloudtrail lookup-events \
  --lookup-attributes AttributeKey=EventName,AttributeValue=GetCallerIdentity \
  --max-results 10 \
  --endpoint-url "$EP"
aws cloudtrail stop-logging --name lab-trail --endpoint-url "$EP"
```

PassRole for the Logs role requires `cloudtrail.amazonaws.com` trust and caller `iam:PassRole`. Skip Logs branch when only testing S3 delivery.

### InjectEvents + ValidateLogs

Requires `NOCTAXRIS_CLOUDTRAIL_INJECT=1` on the API process (default off returns AccessDenied). Seed a forensic line with signed `NoctaxrisCloudTrail.InjectEvents`, then filter with LookupEvents (`SourceIPAddress` / `EventName`). After CreateTrail + StartLogging, call lab `ValidateLogs` against a delivered log object key to assert the digest sidecar (`Valid: true`).

```bash
# API process must have NOCTAXRIS_CLOUDTRAIL_INJECT=1
# Inject via SigV4 POST (X-Amz-Target: NoctaxrisCloudTrail.InjectEvents), then:
aws cloudtrail lookup-events \
  --lookup-attributes AttributeKey=SourceIPAddress,AttributeValue=198.51.100.10 \
  --endpoint-url "$EP"
aws cloudtrail lookup-events \
  --lookup-attributes AttributeKey=EventName,AttributeValue=ConsoleLogin \
  --endpoint-url "$EP"

# After StartLogging, pick a /CloudTrail/ object key (not CloudTrail-Digest) and:
# SigV4 POST CloudTrail_20131101.ValidateLogs
#   {"S3BucketName":"ct-lab","S3ObjectKey":"<log-key>"}
# expect {"Valid":true}
```

SDK suites (Go `workflow_governance_test.go`, Node `workflow.test.mjs`, Python `test_cloudtrail.py`) skip the inject path unless `NOCTAXRIS_CLOUDTRAIL_INJECT=1`.

## Not yet / deferred

- CloudTrail Lake / event data stores
- Insights ML/anomaly engine and PutInsightSelectors trail enablement (lab inject + LookupEvents EventCategory=insight only)
- Full multi-account organization-trail delivery matrix beyond the `IsOrganizationTrail` ARN flag + account-scoped Lookup
- Multi-attribute LookupAttributes (AWS allows one today in this lab as well)
- Event history retention policies beyond the lab JSONL file
- Cross-account LookupEvents
- Broad `resources[]` coverage on every mutating API (key S3/Lambda/Secrets paths enriched)
