"""Prowler AWS enumerate smoke. Soft-skip without prowler or an endpoint env."""

from __future__ import annotations

import os
import shutil
import subprocess

import pytest


def test_prowler_aws_enumerate_starts():
    if shutil.which("prowler") is None:
        pytest.skip("prowler not installed")
    endpoint = (os.environ.get("AWS_ENDPOINT_URL") or os.environ.get("NOCTAXRIS_ENDPOINT") or "").strip()
    if not endpoint:
        pytest.skip("AWS_ENDPOINT_URL unset")

    env = os.environ.copy()
    env["AWS_ENDPOINT_URL"] = endpoint
    env["AWS_EC2_METADATA_DISABLED"] = "true"
    env.setdefault("AWS_DEFAULT_REGION", "us-east-1")
    proc = subprocess.run(
        ["prowler", "aws", "--service", "iam", "--region", "us-east-1"],
        env=env,
        timeout=180,
        check=False,
    )
    # 3 is Prowler's "findings present" code; startup/list must not crash.
    if proc.returncode not in (0, 3):
        pytest.fail(f"prowler aws --service iam exited {proc.returncode}")
