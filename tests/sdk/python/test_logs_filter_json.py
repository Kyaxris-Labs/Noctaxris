"""Logs FilterLogEvents JSON field equality on CloudTrail-shaped messages."""

from __future__ import annotations

import time

from conftest import json_target

CT_ASSUME_ROLE_LOG = (
    '{"eventVersion":"1.08","userIdentity":{"type":"IAMUser",'
    '"principalId":"AIDACKCEVSQ6C2EXAMPLE",'
    '"arn":"arn:aws:iam::000000000001:user/alice"},'
    '"eventTime":"2026-01-02T03:04:05Z","eventSource":"sts.amazonaws.com",'
    '"eventName":"AssumeRole","awsRegion":"us-east-1"}'
)


def test_logs_filter_json_event_name(unique_prefix):
    group = f"/lab/sdk-json-{unique_prefix}"
    stream = "s1"
    ts = int(time.time() * 1000)

    json_target(
        "Logs_20140328.CreateLogGroup",
        "logs",
        {"logGroupName": group},
    )
    try:
        json_target(
            "Logs_20140328.CreateLogStream",
            "logs",
            {"logGroupName": group, "logStreamName": stream},
        )
        json_target(
            "Logs_20140328.PutLogEvents",
            "logs",
            {
                "logGroupName": group,
                "logStreamName": stream,
                "logEvents": [
                    {
                        "timestamp": ts,
                        "message": '{"eventName":"GetObject","eventSource":"s3.amazonaws.com"}',
                    },
                    {"timestamp": ts + 1, "message": CT_ASSUME_ROLE_LOG},
                    {"timestamp": ts + 2, "message": "plain text noise"},
                ],
            },
        )
        filtered = json_target(
            "Logs_20140328.FilterLogEvents",
            "logs",
            {
                "logGroupName": group,
                "filterPattern": '{ $.eventName = "AssumeRole" }',
            },
        )
        events = filtered.get("events") or []
        assert len(events) == 1, filtered
        assert '"eventName":"AssumeRole"' in (events[0].get("message") or "")
    finally:
        try:
            json_target(
                "Logs_20140328.DeleteLogStream",
                "logs",
                {"logGroupName": group, "logStreamName": stream},
            )
        except Exception:
            pass
        try:
            json_target(
                "Logs_20140328.DeleteLogGroup",
                "logs",
                {"logGroupName": group},
            )
        except Exception:
            pass
