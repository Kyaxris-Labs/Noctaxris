"""Lab forensics SetClock + BulkSeed (no FreezeClock)."""

from __future__ import annotations

import os

import pytest

from conftest import json_target


@pytest.mark.skipif(
    os.environ.get("NOCTAXRIS_LAB_FORENSICS") != "1",
    reason="set NOCTAXRIS_LAB_FORENSICS=1 on the API process for NoctaxrisLab SetClock/BulkSeed",
)
def test_forensic_lab_set_clock_and_bulk_seed():
    fixed = "2026-07-20T12:00:00Z"
    set_clock = json_target(
        "NoctaxrisLab.SetClock",
        "noctaxris",
        {"FixedTime": fixed},
    )
    assert set_clock.get("ClockTime") == fixed, set_clock

    seed = json_target(
        "NoctaxrisLab.BulkSeed",
        "noctaxris",
        {"ScenarioId": "crypto-mining", "IncludeGuardDuty": True},
    )
    assert seed.get("ScenarioId") == "crypto-mining", seed
    assert (seed.get("CloudTrailEventCount") or 0) >= 1, seed
    assert (seed.get("GuardDutyFindingCount") or 0) >= 1, seed
