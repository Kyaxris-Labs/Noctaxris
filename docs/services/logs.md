# CloudWatch Logs

**Status:** shipped (lab core)

Lab log groups and streams with Put/GetLogEvents, DescribeLogGroups/DescribeLogStreams, and DeleteLogGroup/DeleteLogStream. Persisted in SQLite. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Groups | `CreateLogGroup`, `DeleteLogGroup`, `DescribeLogGroups` (optional `logGroupNamePrefix`) |
| Streams | `CreateLogStream`, `DeleteLogStream`, `DescribeLogStreams` (optional `logStreamNamePrefix`) |
| Events | `PutLogEvents` (sequence token after first put) and `GetLogEvents` (`startFromHead`, optional time bounds) |
| Subscriptions | `PutSubscriptionFilter` / `DeleteSubscriptionFilter` / `DescribeSubscriptionFilters` to Lambda or SQS. Lab filter pattern is substring match. Fan-out on PutLogEvents (best-effort). Delivery uses the destination ARN owner account. Destination resource policy must Allow `logs.amazonaws.com` (with log-group `aws:SourceArn`). Lambda destinations use the AWS `awslogs.data` gzip+base64 envelope; SQS destinations are lab-only raw `DATA_MESSAGE` JSON. Lambda ignores `roleArn` (resource-policy path). For SQS, optional `roleArn` requires PassRole + `logs.amazonaws.com` trust and AND with destination policy at deliver |
| Metric filters | `PutMetricFilter` / `DeleteMetricFilter` / `DescribeMetricFilters`. `DescribeLogGroups` reports honest `metricFilterCount`. Matching PutLogEvents emit SQLite datapoints readable via store `GetMetricData` (no full CloudWatch Metrics API) |

Log group ARN shape: `arn:aws:logs:REGION:ACCOUNT:log-group:NAME`. Stream ARN adds `:log-stream:STREAM`.

### Authz notes

Identity `EvaluateFull` on `logs:*` actions against the log group or stream ARN (or `*` for DescribeLogGroups). Org SCP/RCP filters apply. No log-group resource policy path. PutSubscriptionFilter with non-empty `roleArn` (SQS lab path) requires `iam:PassRole` plus `logs.amazonaws.com` trust.

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

## Not yet / deferred

- Insights queries, export tasks, FilterLogEvents
- CloudWatch Logs filter syntax (lab uses substring match)
- Full CloudWatch Metrics / Alarms surface (datapoints are store-lite only)
- Log-group resource policies
- Kinesis / Firehose / OpenSearch subscription destinations
- Full pagination token parity
