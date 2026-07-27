"""Detective CreateGraph + SearchGraph seeded via CloudTrail and GuardDuty inject."""

from __future__ import annotations

import os

import pytest

from conftest import json_target


@pytest.mark.skipif(
    os.environ.get("NOCTAXRIS_CLOUDTRAIL_INJECT") != "1"
    or os.environ.get("NOCTAXRIS_GUARDDUTY_INJECT") != "1",
    reason=(
        "requires NOCTAXRIS_CLOUDTRAIL_INJECT=1 and NOCTAXRIS_GUARDDUTY_INJECT=1 "
        "on the API process for SearchGraph seed"
    ),
)
def test_detective_create_graph_search_graph(sts_client, unique_prefix):
    account_id = sts_client.get_caller_identity()["Account"]
    shared_ip = "203.0.113.50"
    role_arn = f"arn:aws:iam::{account_id}:role/SdkDetective-{unique_prefix}"
    event_id = f"sdk-det-{unique_prefix}"

    created = json_target("AmazonDetective.CreateGraph", "detective", {})
    graph_arn = created.get("GraphArn")
    assert graph_arn, f"CreateGraph missing GraphArn: {created}"

    json_target(
        "NoctaxrisCloudTrail.InjectEvents",
        "cloudtrail",
        {
            "Events": [
                {
                    "eventTime": "2026-07-20T12:00:00Z",
                    "sourceIPAddress": shared_ip,
                    "userIdentity": {"type": "IAMUser", "arn": role_arn},
                    "eventSource": "sts.amazonaws.com",
                    "eventName": "AssumeRole",
                    "eventID": event_id,
                    "readOnly": True,
                }
            ]
        },
    )

    detector = json_target("GuardDuty.CreateDetector", "guardduty", {})
    detector_id = detector.get("DetectorId")
    assert detector_id, f"CreateDetector missing DetectorId: {detector}"

    json_target(
        "NoctaxrisGuardDuty.InjectFindings",
        "guardduty",
        {
            "DetectorId": detector_id,
            "Findings": [
                {
                    "type": "UnauthorizedAccess:IAMUser/InstanceCredentialExfiltration.InsideAWS",
                    "severity": 8,
                    "resource": {
                        "resourceType": "AccessKey",
                        "accessKeyDetails": {"userName": "sdk-detective"},
                    },
                    "service": {
                        "action": {
                            "awsApiCallAction": {
                                "remoteIpDetails": {"ipAddressV4": shared_ip}
                            }
                        }
                    },
                }
            ],
        },
    )

    search = json_target(
        "AmazonDetective.SearchGraph",
        "detective",
        {
            "GraphArn": graph_arn,
            "ResourceArn": role_arn,
            "IpAddress": shared_ip,
            "MaxResults": 10,
        },
    )
    assert search.get("CloudTrailEvents"), f"SearchGraph CloudTrailEvents empty: {search}"
    assert search.get("GuardDutyFindings"), f"SearchGraph GuardDutyFindings empty: {search}"
