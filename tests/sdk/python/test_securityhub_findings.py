"""Security Hub BatchImportFindings (ASFF) then GetFindings."""

from __future__ import annotations

from datetime import datetime, timezone

from conftest import json_target


def test_securityhub_batch_import_and_get_findings(account_id, unique_prefix):
    region = "us-east-1"
    finding_id = f"sdk-sh-{unique_prefix}"
    generator_id = "noctaxris-sdk-lab"
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    product_arn = (
        f"arn:aws:securityhub:{region}:{account_id}:product/{account_id}/default"
    )

    imported = json_target(
        "SecurityHub.BatchImportFindings",
        "securityhub",
        {
            "Findings": [
                {
                    "SchemaVersion": "2018-10-08",
                    "Id": finding_id,
                    "ProductArn": product_arn,
                    "GeneratorId": generator_id,
                    "AwsAccountId": account_id,
                    "Types": [
                        "Software and Configuration Checks/Vulnerabilities/CVE"
                    ],
                    "CreatedAt": now,
                    "UpdatedAt": now,
                    "Severity": {"Label": "HIGH"},
                    "Title": "sdk lab finding",
                    "Description": "security hub sdk import",
                    "Resources": [
                        {
                            "Type": "AwsS3Bucket",
                            "Id": "arn:aws:s3:::sdk-sh-lab",
                        }
                    ],
                }
            ]
        },
    )
    assert imported.get("SuccessCount") == 1
    assert imported.get("FailedCount") == 0

    got = json_target(
        "SecurityHub.GetFindings",
        "securityhub",
        {
            "GeneratorId": generator_id,
            "SeverityLabel": "HIGH",
            "ResourceType": "AwsS3Bucket",
        },
    )
    findings = got.get("Findings") or []
    assert any(
        f.get("Id") == finding_id or f.get("Title") == "sdk lab finding"
        for f in findings
    ), f"GetFindings missing imported finding: {got}"
