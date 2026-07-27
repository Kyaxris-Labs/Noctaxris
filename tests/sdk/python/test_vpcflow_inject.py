"""VPC Flow CreateFlowLogs + InjectFlowLogs to S3 with ACCEPT/REJECT v2 lines."""

from __future__ import annotations

import os

import pytest

from conftest import json_target


@pytest.mark.skipif(
    os.environ.get("NOCTAXRIS_VPCFLOW_INJECT") != "1",
    reason="set NOCTAXRIS_VPCFLOW_INJECT=1 on the API process for lab InjectFlowLogs",
)
def test_vpcflow_create_inject_s3_accept_reject(s3_client, unique_prefix):
    bucket = f"{unique_prefix}-vpcflow".lower()
    s3_client.create_bucket(Bucket=bucket)
    try:
        created = json_target(
            "AmazonEC2.CreateFlowLogs",
            "ec2",
            {
                "ResourceIds": ["vpc-labopaque001"],
                "ResourceType": "VPC",
                "TrafficType": "ALL",
                "LogDestinationType": "s3",
                "LogDestination": f"arn:aws:s3:::{bucket}",
            },
        )
        flow_log_ids = created.get("FlowLogIds") or []
        assert len(flow_log_ids) == 1, created
        flow_log_id = flow_log_ids[0]
        assert str(flow_log_id).startswith("fl-"), flow_log_id

        json_target(
            "NoctaxrisEC2.InjectFlowLogs",
            "ec2",
            {"FlowLogId": flow_log_id},
        )

        listed = s3_client.list_objects_v2(Bucket=bucket, Prefix="AWSLogs/")
        contents = listed.get("Contents") or []
        assert contents, "no flow log object in S3"
        body = s3_client.get_object(Bucket=bucket, Key=contents[0]["Key"])[
            "Body"
        ].read()
        text = body.decode("utf-8", errors="replace")
        assert " ACCEPT OK" in text, text
        assert " REJECT OK" in text, text
    finally:
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
