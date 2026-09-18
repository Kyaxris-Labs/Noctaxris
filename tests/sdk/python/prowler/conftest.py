"""Prowler smoke skips without the binary or an endpoint env (no fail-closed ready check)."""

from __future__ import annotations

import os
import shutil

import pytest


@pytest.fixture(scope="session", autouse=True)
def _api_ready() -> None:
    if shutil.which("prowler") is None:
        pytest.skip("prowler not installed")
    endpoint = (os.environ.get("AWS_ENDPOINT_URL") or os.environ.get("NOCTAXRIS_ENDPOINT") or "").strip()
    if not endpoint:
        pytest.skip("AWS_ENDPOINT_URL unset")
