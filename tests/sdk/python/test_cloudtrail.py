"""CloudTrail CreateTrail + StartLogging S3 delivery; lab InjectEvents + ValidateLogs."""

from __future__ import annotations

import os

import pytest

from conftest import json_target


def test_cloudtrail_create_start_s3_delivery(
    s3_client, cloudtrail_client, account_id, unique_prefix
):
    bucket = f"{unique_prefix}-ct".lower()
    trail = f"{unique_prefix}-trail"

    s3_client.create_bucket(Bucket=bucket)
    try:
        cloudtrail_client.create_trail(Name=trail, S3BucketName=bucket)
        cloudtrail_client.start_logging(Name=trail)

        listed = s3_client.list_objects_v2(
            Bucket=bucket, Prefix=f"AWSLogs/{account_id}/CloudTrail/"
        )
        contents = listed.get("Contents") or []
        keys = [
            o["Key"]
            for o in contents
            if f"noctaxris-{trail}-" in o["Key"] or "CloudTrail/" in o["Key"]
        ]
        assert keys, f"missing CloudTrail delivery object: {contents}"
        body = s3_client.get_object(Bucket=bucket, Key=keys[0])["Body"].read()
        assert body
    finally:
        try:
            cloudtrail_client.stop_logging(Name=trail)
        except Exception:
            pass
        try:
            cloudtrail_client.delete_trail(Name=trail)
        except Exception:
            pass
        try:
            listed = s3_client.list_objects_v2(Bucket=bucket)
            for obj in listed.get("Contents") or []:
                s3_client.delete_object(Bucket=bucket, Key=obj["Key"])
        except Exception:
            pass
        try:
            s3_client.delete_bucket(Bucket=bucket)
        except Exception:
            pass


@pytest.mark.skipif(
    os.environ.get("NOCTAXRIS_CLOUDTRAIL_INJECT") != "1",
    reason="set NOCTAXRIS_CLOUDTRAIL_INJECT=1 on the API process for lab InjectEvents",
)
def test_cloudtrail_inject_lookup_and_validate_logs(
    s3_client, cloudtrail_client, account_id, unique_prefix
):
    bucket = f"{unique_prefix}-ctinj".lower()
    trail = f"{unique_prefix}-inj"
    event_id = f"sdk-inj-{unique_prefix}"
    source_ip = "198.51.100.44"

    json_target(
        "NoctaxrisCloudTrail.InjectEvents",
        "cloudtrail",
        {
            "Events": [
                {
                    "eventTime": "2026-07-20T15:00:00Z",
                    "sourceIPAddress": source_ip,
                    "userIdentity": {"type": "IAMUser", "userName": "sdk-forensic"},
                    "eventSource": "signin.amazonaws.com",
                    "eventName": "ConsoleLogin",
                    "eventID": event_id,
                    "readOnly": False,
                }
            ]
        },
    )

    by_ip = cloudtrail_client.lookup_events(
        LookupAttributes=[
            {"AttributeKey": "SourceIPAddress", "AttributeValue": source_ip}
        ],
        MaxResults=10,
    )
    assert any(
        ev.get("EventId") == event_id
        or event_id in (ev.get("CloudTrailEvent") or "")
        for ev in by_ip.get("Events") or []
    ), f"LookupEvents SourceIPAddress missing {event_id}"

    by_name = cloudtrail_client.lookup_events(
        LookupAttributes=[
            {"AttributeKey": "EventName", "AttributeValue": "ConsoleLogin"}
        ],
        MaxResults=10,
    )
    assert any(
        ev.get("EventId") == event_id
        or event_id in (ev.get("CloudTrailEvent") or "")
        for ev in by_name.get("Events") or []
    ), f"LookupEvents EventName missing {event_id}"

    s3_client.create_bucket(Bucket=bucket)
    try:
        cloudtrail_client.create_trail(Name=trail, S3BucketName=bucket)
        cloudtrail_client.start_logging(Name=trail)

        listed = s3_client.list_objects_v2(Bucket=bucket, Prefix="AWSLogs/")
        contents = listed.get("Contents") or []
        log_key = next(
            (
                o["Key"]
                for o in contents
                if "/CloudTrail/" in o["Key"]
                and "CloudTrail-Digest" not in o["Key"]
            ),
            None,
        )
        assert log_key, f"missing CloudTrail log object: {contents}"

        validated = json_target(
            "CloudTrail_20131101.ValidateLogs",
            "cloudtrail",
            {"S3BucketName": bucket, "S3ObjectKey": log_key},
        )
        assert validated.get("Valid") is True, validated
    finally:
        try:
            cloudtrail_client.stop_logging(Name=trail)
        except Exception:
            pass
        try:
            cloudtrail_client.delete_trail(Name=trail)
        except Exception:
            pass
        try:
            listed = s3_client.list_objects_v2(Bucket=bucket)
            for obj in listed.get("Contents") or []:
                s3_client.delete_object(Bucket=bucket, Key=obj["Key"])
        except Exception:
            pass
        try:
            s3_client.delete_bucket(Bucket=bucket)
        except Exception:
            pass
