"""Macie EnableMacie + InjectFindings (S3Objects) + List/GetFindings."""

from __future__ import annotations

import os

import pytest

from conftest import json_target


@pytest.mark.skipif(
    os.environ.get("NOCTAXRIS_MACIE_INJECT") != "1",
    reason="set NOCTAXRIS_MACIE_INJECT=1 on the API process for lab InjectFindings",
)
def test_macie_enable_inject_list_get_findings(s3_client, unique_prefix):
    bucket = f"{unique_prefix}-macie".lower()
    key = "pii.csv"
    body = b"ssn 987-65-4321\n"

    json_target("Macie2.EnableMacie", "macie2", {})

    s3_client.create_bucket(Bucket=bucket)
    try:
        s3_client.put_object(Bucket=bucket, Key=key, Body=body)

        inj = json_target(
            "NoctaxrisMacie.InjectFindings",
            "macie2",
            {"S3Objects": [{"Bucket": bucket, "Key": key}]},
        )
        finding_ids = inj.get("findingIds") or []
        assert finding_ids, f"InjectFindings findingIds empty: {inj}"

        listed = json_target("Macie2.ListFindings", "macie2", {})
        listed_ids = listed.get("findingIds") or []
        assert finding_ids[0] in listed_ids, f"ListFindings missing {finding_ids[0]}: {listed}"

        got = json_target(
            "Macie2.GetFindings",
            "macie2",
            {"findingIds": finding_ids},
        )
        findings = got.get("findings") or []
        assert findings, f"GetFindings findings empty: {got}"
        raw = str(got)
        assert "SensitiveData" in raw, f"GetFindings missing SensitiveData: {got}"
    finally:
        try:
            s3_client.delete_object(Bucket=bucket, Key=key)
        except Exception:
            pass
        try:
            s3_client.delete_bucket(Bucket=bucket)
        except Exception:
            pass
