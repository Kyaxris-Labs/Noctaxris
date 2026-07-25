"""Firehose OpenSearch destination; skip when domain not Active."""

from __future__ import annotations

import time

import pytest
from conftest import FIREHOSE_TRUST, json_target


def test_firehose_opensearch_describe_skip_without_engine(
    iam_client, firehose_client, account_id, unique_prefix
):
    domain = f"{unique_prefix}-fh-os".replace("_", "-")[:28]
    stream = f"{unique_prefix}-fh"[:64]
    role_name = f"{unique_prefix}-fh-os-role"

    role = iam_client.create_role(
        RoleName=role_name,
        AssumeRolePolicyDocument=FIREHOSE_TRUST,
    )
    role_arn = role["Role"]["Arn"]
    iam_client.put_role_policy(
        RoleName=role_name,
        PolicyName="es-put",
        PolicyDocument=(
            '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
            '"Action":"es:ESHttpPut","Resource":"*"}]}'
        ),
    )

    try:
        json_target(
            "AmazonOpenSearchService.CreateDomain",
            "es",
            {"DomainName": domain, "EngineVersion": "OpenSearch_2.11"},
            content_type="application/x-amz-json-1.0",
        )

        status = ""
        endpoint_s = ""
        for _ in range(8):
            desc = json_target(
                "AmazonOpenSearchService.DescribeDomain",
                "es",
                {"DomainName": domain},
                content_type="application/x-amz-json-1.0",
            )
            st = desc.get("DomainStatus") or {}
            if isinstance(st, dict):
                status = st.get("DomainStatus") or (
                    "Active" if st.get("Created") else ""
                )
                endpoint_s = str(st.get("Endpoint") or "")
                if status in ("Active", "CreateFailed") or endpoint_s.startswith(
                    "stub://"
                ):
                    break
            time.sleep(0.5)

        if status != "Active" or endpoint_s.startswith("stub://"):
            pytest.skip(
                f"OpenSearch not Active for Firehose dest (status={status!r})"
            )

        domain_arn = f"arn:aws:es:us-east-1:{account_id}:domain/{domain}"
        firehose_client.create_delivery_stream(
            DeliveryStreamName=stream,
            AmazonopensearchserviceDestinationConfiguration={
                "DomainARN": domain_arn,
                "IndexName": "events",
                "RoleARN": role_arn,
            },
        )
        try:
            got = firehose_client.describe_delivery_stream(
                DeliveryStreamName=stream
            )
            dests = got["DeliveryStreamDescription"]["Destinations"]
            assert dests
            os_desc = dests[0].get(
                "AmazonopensearchserviceDestinationDescription"
            ) or dests[0].get("OpenSearchDestinationDescription")
            assert os_desc
            assert os_desc.get("DomainARN") or os_desc.get("IndexName")
            assert os_desc.get("IndexName") == "events"
            assert os_desc.get("RoleARN") == role_arn
        finally:
            try:
                firehose_client.delete_delivery_stream(
                    DeliveryStreamName=stream
                )
            except Exception:
                pass
    finally:
        try:
            json_target(
                "AmazonOpenSearchService.DeleteDomain",
                "es",
                {"DomainName": domain},
                content_type="application/x-amz-json-1.0",
            )
        except Exception:
            pass
        try:
            iam_client.delete_role_policy(RoleName=role_name, PolicyName="es-put")
        except Exception:
            pass
        try:
            iam_client.delete_role(RoleName=role_name)
        except Exception:
            pass
