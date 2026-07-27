"""CloudWatch Metrics + Alarms smoke via GraniteService JSON."""

from __future__ import annotations

from datetime import datetime, timedelta, timezone

from conftest import json_target


def test_cloudwatch_metrics_alarms_smoke(unique_prefix):
    now = datetime.now(timezone.utc)
    start = (now - timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ")
    end = (now + timedelta(minutes=1)).strftime("%Y-%m-%dT%H:%M:%SZ")
    ns = f"Sdk/MyApp-{unique_prefix}"
    alarm_name = f"sdk-alarm-{unique_prefix}"

    json_target(
        "GraniteServiceVersion20100801.PutMetricData",
        "monitoring",
        {
            "Namespace": ns,
            "MetricData": [
                {
                    "MetricName": "RequestCount",
                    "Value": 7.0,
                    "Unit": "Count",
                    "Timestamp": int(now.timestamp()),
                    "Dimensions": [{"Name": "Service", "Value": "api"}],
                }
            ],
        },
        content_type="application/x-amz-json-1.0",
    )

    listed = json_target(
        "GraniteServiceVersion20100801.ListMetrics",
        "monitoring",
        {"Namespace": ns},
        content_type="application/x-amz-json-1.0",
    )
    assert len(listed.get("Metrics") or []) == 1, listed

    stats = json_target(
        "GraniteServiceVersion20100801.GetMetricStatistics",
        "monitoring",
        {
            "Namespace": ns,
            "MetricName": "RequestCount",
            "StartTime": start,
            "EndTime": end,
            "Period": 60,
            "Statistics": ["Sum"],
            "Dimensions": [{"Name": "Service", "Value": "api"}],
        },
        content_type="application/x-amz-json-1.0",
    )
    dps = stats.get("Datapoints") or []
    assert len(dps) > 0, stats
    assert float(dps[0].get("Sum") or 0) >= 7, dps[0]

    json_target(
        "GraniteServiceVersion20100801.PutMetricAlarm",
        "monitoring",
        {
            "AlarmName": alarm_name,
            "MetricName": "RequestCount",
            "Namespace": ns,
            "Statistic": "Sum",
            "Period": 60,
            "EvaluationPeriods": 1,
            "Threshold": 1.0,
            "ComparisonOperator": "GreaterThanThreshold",
            "Dimensions": [{"Name": "Service", "Value": "api"}],
        },
        content_type="application/x-amz-json-1.0",
    )

    desc = json_target(
        "GraniteServiceVersion20100801.DescribeAlarms",
        "monitoring",
        {"AlarmNames": [alarm_name]},
        content_type="application/x-amz-json-1.0",
    )
    assert len(desc.get("MetricAlarms") or []) == 1, desc

    json_target(
        "GraniteServiceVersion20100801.SetAlarmState",
        "monitoring",
        {
            "AlarmName": alarm_name,
            "StateValue": "ALARM",
            "StateReason": "sdk smoke",
        },
        content_type="application/x-amz-json-1.0",
    )

    after = json_target(
        "GraniteServiceVersion20100801.DescribeAlarms",
        "monitoring",
        {"AlarmNames": [alarm_name]},
        content_type="application/x-amz-json-1.0",
    )
    assert (after.get("MetricAlarms") or [{}])[0].get("StateValue") == "ALARM", after

    json_target(
        "GraniteServiceVersion20100801.DeleteAlarms",
        "monitoring",
        {"AlarmNames": [alarm_name]},
        content_type="application/x-amz-json-1.0",
    )
