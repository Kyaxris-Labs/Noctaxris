"""Shared fixtures for Noctaxris boto3 integration tests."""

from __future__ import annotations

import os
import time
import urllib.error
import urllib.request

import boto3
import pytest
from botocore.client import Config


def endpoint() -> str:
    return os.environ.get("NOCTAXRIS_ENDPOINT", "http://127.0.0.1:4566").rstrip("/")


def skip_if_down_enabled() -> bool:
    return os.environ.get("NOCTAXRIS_SKIP_IF_DOWN", "").strip().lower() in (
        "1",
        "true",
        "yes",
    )


def require_ready() -> None:
    """Fail (or skip if NOCTAXRIS_SKIP_IF_DOWN) when the API is unreachable."""
    ep = endpoint()
    url = f"{ep}/_noctaxris/ready"
    try:
        with urllib.request.urlopen(url, timeout=2) as resp:
            if resp.status != 200:
                msg = f"Noctaxris not ready at {ep}: status {resp.status}"
                if skip_if_down_enabled():
                    pytest.skip(msg)
                pytest.fail(msg)
    except (urllib.error.URLError, TimeoutError, OSError) as err:
        msg = f"Noctaxris not reachable at {ep}: {err}"
        if skip_if_down_enabled():
            pytest.skip(msg)
        pytest.fail(msg)


@pytest.fixture(scope="session", autouse=True)
def _api_ready() -> None:
    require_ready()


@pytest.fixture
def unique_prefix() -> str:
    return f"pyit-{time.time_ns()}"


def _creds() -> dict:
    return {
        "aws_access_key_id": os.environ.get("AWS_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01"),
        "aws_secret_access_key": os.environ.get(
            "AWS_SECRET_ACCESS_KEY",
            "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
        ),
        "region_name": os.environ.get("AWS_DEFAULT_REGION", "us-east-1"),
        "endpoint_url": endpoint(),
    }


@pytest.fixture
def sts_client():
    return boto3.client("sts", **_creds())


@pytest.fixture
def s3_client():
    return boto3.client(
        "s3",
        **_creds(),
        config=Config(s3={"addressing_style": "path"}),
    )


@pytest.fixture
def dynamodb_client():
    return boto3.client("dynamodb", **_creds())


@pytest.fixture
def iam_client():
    return boto3.client("iam", **_creds())


@pytest.fixture
def kms_client():
    return boto3.client("kms", **_creds())


@pytest.fixture
def sqs_client():
    return boto3.client("sqs", **_creds())


@pytest.fixture
def sns_client():
    return boto3.client("sns", **_creds())


@pytest.fixture
def lambda_client():
    return boto3.client("lambda", **_creds())


@pytest.fixture
def events_client():
    return boto3.client("events", **_creds())


@pytest.fixture
def ssm_client():
    return boto3.client("ssm", **_creds())


@pytest.fixture
def secretsmanager_client():
    return boto3.client("secretsmanager", **_creds())
