# CloudWatch Metrics and Alarms

**Status:** shipped (lab core)

Custom metrics and metric alarms over JSON (`X-Amz-Target: GraniteServiceVersion20100801.*`, SigV4 service `monitoring`). CloudWatch Logs stay in [logs.md](logs.md).

## Implemented

| Area | Actions |
|------|---------|
| Metrics | `PutMetricData`, `ListMetrics`, `GetMetricStatistics`, `GetMetricData` (MetricStat only) |
| Alarms | `PutMetricAlarm`, `DescribeAlarms`, `DeleteAlarms`, `SetAlarmState` |

### Authz notes

Identity `EvaluateFull` on `cloudwatch:*` against `*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
export AWS_ENDPOINT_URL="$EP"

aws cloudwatch put-metric-data \
  --namespace MyApp \
  --metric-data 'MetricName=RequestCount,Value=42,Unit=Count,Dimensions=[{Name=Service,Value=api}]' \
  --endpoint-url "$EP"

aws cloudwatch list-metrics --namespace MyApp --endpoint-url "$EP"

aws cloudwatch get-metric-statistics \
  --namespace MyApp \
  --metric-name RequestCount \
  --dimensions Name=Service,Value=api \
  --start-time "$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-1H +%Y-%m-%dT%H:%M:%SZ)" \
  --end-time "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --period 60 \
  --statistics Sum Average \
  --endpoint-url "$EP"

aws cloudwatch put-metric-alarm \
  --alarm-name high-requests \
  --metric-name RequestCount \
  --namespace MyApp \
  --statistic Sum \
  --period 60 \
  --threshold 10 \
  --comparison-operator GreaterThanThreshold \
  --evaluation-periods 1 \
  --dimensions Name=Service,Value=api \
  --endpoint-url "$EP"

aws cloudwatch describe-alarms --alarm-names high-requests --endpoint-url "$EP"
aws cloudwatch set-alarm-state \
  --alarm-name high-requests \
  --state-value ALARM \
  --state-reason "lab manual" \
  --endpoint-url "$EP"
aws cloudwatch delete-alarms --alarm-names high-requests --endpoint-url "$EP"
```

Live Compose smoke skipped when Docker is unavailable. SDK Go/Node/Python metrics+alarms rows soft-skip when the API is down. Terraform omits `aws_cloudwatch_metric_alarm` (provider Query vs Granite JSON); use SDK. Related DDB LSI stack: `STACK=lab-parity-observe` (`TF_PARITY=1`).

## Not yet / deferred

- Metric math expressions in `GetMetricData` (MetricStat pass-through only)
- Composite alarms, anomaly detection, Contributor Insights, dashboards
- Alarm action fan-out to SNS/Auto Scaling (actions stored only when present on Put; not evaluated)
- Cross-account metrics, metric streams, `GetMetricWidgetImage`
- smithy-rpc-v2-cbor wire protocol (JSON `GraniteServiceVersion20100801.*` and Query-capable botocore JSON path are supported)
