"""GuardDuty CreateDetector + lab InjectFindings + List/GetFindings."""

from __future__ import annotations

import os

import pytest

from conftest import json_target


@pytest.mark.skipif(
    os.environ.get("NOCTAXRIS_GUARDDUTY_INJECT") != "1",
    reason="set NOCTAXRIS_GUARDDUTY_INJECT=1 on the API process for lab InjectFindings",
)
def test_guardduty_create_inject_list_get_findings():
    created = json_target("GuardDuty.CreateDetector", "guardduty", {})
    detector_id = created.get("DetectorId")
    assert detector_id, f"CreateDetector missing DetectorId: {created}"

    injected = json_target(
        "NoctaxrisGuardDuty.InjectFindings",
        "guardduty",
        {
            "DetectorId": detector_id,
            "Findings": [
                {
                    "type": (
                        "UnauthorizedAccess:IAMUser/"
                        "InstanceCredentialExfiltration.InsideAWS"
                    ),
                    "severity": 8,
                    "title": "sdk cred exfil",
                }
            ],
        },
    )
    finding_ids = injected.get("FindingIds") or []
    assert len(finding_ids) == 1, f"FindingIds={finding_ids}"
    finding_id = finding_ids[0]
    assert finding_id

    listed = json_target(
        "GuardDuty.ListFindings",
        "guardduty",
        {"DetectorId": detector_id},
    )
    assert finding_id in (listed.get("FindingIds") or []), listed

    got = json_target(
        "GuardDuty.GetFindings",
        "guardduty",
        {"DetectorId": detector_id, "FindingIds": [finding_id]},
    )
    findings = got.get("Findings") or []
    assert len(findings) == 1
    assert findings[0].get("type"), f"GetFindings missing type: {got}"
