# CloudWatch Logs

**Status:** shipped (lab core)

Lab log groups and streams with Put/GetLogEvents, FilterLogEvents, DescribeLogGroups/DescribeLogStreams, and DeleteLogGroup/DeleteLogStream. Persisted in SQLite. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Groups | `CreateLogGroup`, `DeleteLogGroup`, `DescribeLogGroups` (optional `logGroupNamePrefix`; reports `retentionInDays` when set), `PutRetentionPolicy` / `DeleteRetentionPolicy` (AWS-allowed day values; expired events purged on put/get/describe) |
| Streams | `CreateLogStream`, `DeleteLogStream`, `DescribeLogStreams` (optional `logStreamNamePrefix`) |
| Events | `PutLogEvents` (sequence token after first put), `GetLogEvents` (`startFromHead`, optional time bounds), and `FilterLogEvents` (optional `logStreamNames`, `startTime`/`endTime`, lab `filterPattern` subset below, offset `nextToken`; lab page cap 1000) |
| Subscriptions | `PutSubscriptionFilter` / `DeleteSubscriptionFilter` / `DescribeSubscriptionFilters` to Lambda or SQS. Same lab `filterPattern` subset as `FilterLogEvents` (unsupported patterns rejected at put). Fan-out on PutLogEvents (best-effort). Delivery uses the destination ARN owner account. Destination resource policy must Allow `logs.amazonaws.com` (with log-group `aws:SourceArn`). Lambda destinations use the AWS `awslogs.data` gzip+base64 envelope; SQS destinations are lab-only raw `DATA_MESSAGE` JSON. Lambda ignores `roleArn` (resource-policy path). For SQS, optional `roleArn` requires PassRole + `logs.amazonaws.com` trust and AND with destination policy at deliver |
| Metric filters | `PutMetricFilter` / `DeleteMetricFilter` / `DescribeMetricFilters`. Same lab `filterPattern` subset. `DescribeLogGroups` reports honest `metricFilterCount`. Matching PutLogEvents emit SQLite datapoints readable via store `GetMetricData` (no full CloudWatch Metrics API) |
| Resource policy | Account-scoped `PutResourcePolicy` / `GetResourcePolicy` / `DeleteResourcePolicy` / `DescribeResourcePolicies` (AWS Logs shape; soft cap 10). EventBridge RoleArn-less Logs targets require Allow for `events.amazonaws.com` on `logs:PutLogEvents` / `logs:CreateLogStream` (empty policy skips delivery) |

### Lab filter pattern subset

Shared by `FilterLogEvents`, subscription filters, and metric filters. Case-sensitive. Matching is substring-within-message (not AWS word/token boundaries).

| Syntax | Behavior |
|--------|----------|
| (empty) | Match all events |
| `term` | Include: message contains term |
| `a b` | AND: message contains each term |
| `"exact phrase"` | Include exact substring |
| `-term` / `-"phrase"` | Exclude: fail if message contains term/phrase |
| `?` / `*` inside an unquoted term | Single-character / any-run wildcards within that term |

Unsupported patterns return `ValidationException` (no silent fall-through): JSON `{$.field=…}`, space-delimited `[…]`, `%regex%`, `&&` / `||`, and Insights `|` query syntax. This is not CloudWatch Logs Insights (`StartQuery` / `GetQueryResults`).

Log group ARN shape: `arn:aws:logs:REGION:ACCOUNT:log-group:NAME`. Stream ARN adds `:log-stream:STREAM`.

### Authz notes

Identity `EvaluateFull` on `logs:*` actions against the log group or stream ARN (or `*` for DescribeLogGroups / resource-policy APIs). `FilterLogEvents` authorizes on the log group ARN. Org SCP/RCP filters apply. Account resource policies gate EventBridge service-principal delivery; empty policy denies. PutSubscriptionFilter with non-empty `roleArn` (SQS lab path) requires `iam:PassRole` plus `logs.amazonaws.com` trust.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
GROUP="/noctaxris-lab-$RANDOM"
STREAM="s1"

aws logs create-log-group --log-group-name "$GROUP" --endpoint-url "$EP"
aws logs create-log-stream \
  --log-group-name "$GROUP" \
  --log-stream-name "$STREAM" \
  --endpoint-url "$EP"

TS=$(date +%s000)
aws logs put-log-events \
  --log-group-name "$GROUP" \
  --log-stream-name "$STREAM" \
  --log-events "timestamp=$TS,message=hello-logs" \
  --endpoint-url "$EP"

aws logs get-log-events \
  --log-group-name "$GROUP" \
  --log-stream-name "$STREAM" \
  --start-from-head \
  --endpoint-url "$EP"

aws logs filter-log-events \
  --log-group-name "$GROUP" \
  --filter-pattern "hello -bye" \
  --endpoint-url "$EP"

aws logs put-retention-policy \
  --log-group-name "$GROUP" \
  --retention-in-days 7 \
  --endpoint-url "$EP"

aws logs describe-log-groups \
  --log-group-name-prefix /noctaxris \
  --endpoint-url "$EP"

aws logs describe-log-streams \
  --log-group-name "$GROUP" \
  --endpoint-url "$EP"

aws logs delete-log-stream \
  --log-group-name "$GROUP" \
  --log-stream-name "$STREAM" \
  --endpoint-url "$EP"

aws logs delete-log-group --log-group-name "$GROUP" --endpoint-url "$EP"
```

## Out of lab scope

- Insights queries (`StartQuery` / `GetQueryResults` / query language), export tasks (out of lab scope; no fake Insights engine)
- Full CloudWatch Logs filter syntax beyond the lab subset (JSON object filters, space-delimited field patterns, `%regex%`, AWS optional `?term` OR semantics) (out of lab scope; lab filterPattern subset is shipped)
- Full CloudWatch Metrics / Alarms surface (out of lab scope; datapoints are store-lite only)
- Kinesis / Firehose / OpenSearch subscription destinations (out of lab scope)
- Full pagination token parity (`FilterLogEvents` uses a lab offset token) (out of lab scope)
