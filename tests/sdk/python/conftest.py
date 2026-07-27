"""Shared fixtures for Noctaxris boto3 integration tests."""

from __future__ import annotations

import io
import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile
from typing import Any
from urllib.error import HTTPError

import boto3
import pytest
from botocore.auth import SigV4Auth
from botocore.awsrequest import AWSRequest
from botocore.client import Config
from botocore.credentials import Credentials


def endpoint() -> str:
    return os.environ.get("NOCTAXRIS_ENDPOINT", "http://127.0.0.1:4566").rstrip("/")


def skip_if_down_enabled() -> bool:
    return os.environ.get("NOCTAXRIS_SKIP_IF_DOWN", "").strip().lower() in (
        "1",
        "true",
        "yes",
    )


def nested_enabled() -> bool:
    return os.environ.get("NOCTAXRIS_NESTED", "").strip().lower() in (
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


def _access_key() -> str:
    return os.environ.get("AWS_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")


def _secret_key() -> str:
    return os.environ.get(
        "AWS_SECRET_ACCESS_KEY",
        "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
    )


def _region() -> str:
    return os.environ.get("AWS_DEFAULT_REGION", "us-east-1")


def _creds() -> dict:
    return {
        "aws_access_key_id": _access_key(),
        "aws_secret_access_key": _secret_key(),
        "region_name": _region(),
        "endpoint_url": endpoint(),
    }


def _botocore_credentials() -> Credentials:
    return Credentials(_access_key(), _secret_key())


def signed_request(
    method: str,
    url: str,
    *,
    service: str,
    body: bytes = b"",
    headers: dict[str, str] | None = None,
    content_type: str | None = None,
) -> tuple[int, bytes, dict[str, str]]:
    """SigV4-signed HTTP request via urllib (lab JSON/REST facades)."""
    hdrs = dict(headers or {})
    if content_type is not None:
        hdrs["Content-Type"] = content_type
    req = AWSRequest(method=method, url=url, data=body, headers=hdrs)
    SigV4Auth(_botocore_credentials(), service, _region()).add_auth(req)
    prepared = req.prepare()
    http_req = urllib.request.Request(
        prepared.url,
        data=body if method.upper() not in ("GET", "HEAD") else None,
        headers=dict(prepared.headers),
        method=method.upper(),
    )
    try:
        with urllib.request.urlopen(http_req, timeout=30) as resp:
            return resp.status, resp.read(), dict(resp.headers)
    except HTTPError as err:
        return err.code, err.read(), dict(err.headers)


def json_target(
    target: str,
    service: str,
    payload: dict[str, Any] | None = None,
    *,
    content_type: str = "application/x-amz-json-1.1",
) -> dict[str, Any]:
    """POST application/x-amz-json + X-Amz-Target (lab CloudFront/Route53/ELBv2/etc.)."""
    body = json.dumps(payload or {}).encode("utf-8")
    status, raw, _ = signed_request(
        "POST",
        endpoint() + "/",
        service=service,
        body=body,
        headers={"X-Amz-Target": target},
        content_type=content_type,
    )
    if status < 200 or status >= 300:
        raise AssertionError(f"{target} status={status} body={raw!r}")
    if not raw:
        return {}
    return json.loads(raw.decode("utf-8"))


def form_action(service: str, params: dict[str, str]) -> bytes:
    """POST application/x-www-form-urlencoded (lab Config query shape)."""
    body = urllib.parse.urlencode(params).encode("utf-8")
    status, raw, _ = signed_request(
        "POST",
        endpoint() + "/",
        service=service,
        body=body,
        content_type="application/x-www-form-urlencoded",
    )
    if status < 200 or status >= 300:
        raise AssertionError(f"form {params.get('Action')} status={status} body={raw!r}")
    return raw


def minimal_python_zip() -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as zf:
        zf.writestr(
            "index.py",
            "def handler(event, context):\n    return {'ok': True}\n",
        )
    return buf.getvalue()


LAMBDA_TRUST = (
    '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
    '"Principal":{"Service":"lambda.amazonaws.com"},'
    '"Action":"sts:AssumeRole"}]}'
)

APPSYNC_TRUST = (
    '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
    '"Principal":{"Service":"appsync.amazonaws.com"},'
    '"Action":"sts:AssumeRole"}]}'
)

FIREHOSE_TRUST = (
    '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
    '"Principal":{"Service":"firehose.amazonaws.com"},'
    '"Action":"sts:AssumeRole"}]}'
)


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
def kinesis_client():
    return boto3.client("kinesis", **_creds())


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


@pytest.fixture
def apigatewayv2_client():
    return boto3.client("apigatewayv2", **_creds())


@pytest.fixture
def apigateway_client():
    return boto3.client("apigateway", **_creds())


@pytest.fixture
def cognito_idp_client():
    return boto3.client("cognito-idp", **_creds())


@pytest.fixture
def transfer_client():
    return boto3.client("transfer", **_creds())


@pytest.fixture
def glue_client():
    return boto3.client("glue", **_creds())


@pytest.fixture
def stepfunctions_client():
    return boto3.client("stepfunctions", **_creds())


@pytest.fixture
def cloudtrail_client():
    return boto3.client("cloudtrail", **_creds())


@pytest.fixture
def firehose_client():
    return boto3.client("firehose", **_creds())


@pytest.fixture
def account_id(sts_client) -> str:
    return sts_client.get_caller_identity()["Account"]
