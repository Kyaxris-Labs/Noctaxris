# CloudWatch Logs

**Status:** shipped (lab core)

Lab log groups and streams with Put/GetLogEvents and DescribeLogGroups. Persisted in SQLite. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Groups | `CreateLogGroup`, `DescribeLogGroups` (optional `logGroupNamePrefix`) |
| Streams | `CreateLogStream` |
| Events | `PutLogEvents` (sequence token after first put) and `GetLogEvents` (`startFromHead`, optional time bounds) |

Log group ARN shape: `arn:aws:logs:REGION:ACCOUNT:log-group:NAME`. Stream ARN adds `:log-stream:STREAM`.

### Authz notes

Identity `EvaluateFull` on `logs:*` actions against the log group or stream ARN (or `*` for DescribeLogGroups). Org SCP/RCP filters apply. No resource policy path.

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
```

## Not yet / deferred

- Subscriptions, metric filters, Insights queries, export tasks
- DeleteLogGroup / DeleteLogStream / DescribeLogStreams / FilterLogEvents
- Cross-account observability and resource policies
- Full pagination token parity
