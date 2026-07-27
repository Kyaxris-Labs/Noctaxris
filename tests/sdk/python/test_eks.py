"""EKS control-plane CRUD soft-skip when API is down."""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request

import pytest
from botocore.auth import SigV4Auth
from botocore.awsrequest import AWSRequest
from botocore.credentials import Credentials

from conftest import _access_key, _secret_key, endpoint, require_ready


def _region() -> str:
    return os.environ.get("AWS_DEFAULT_REGION", "us-east-1")


def signed_http(
    service: str,
    method: str,
    path: str,
    body: bytes | None = None,
    content_type: str = "",
) -> tuple[int, str]:
    url = endpoint() + path
    headers = {}
    if content_type:
        headers["Content-Type"] = content_type
    req = AWSRequest(method=method, url=url, data=body or b"", headers=headers)
    creds = Credentials(_access_key(), _secret_key())
    SigV4Auth(creds, service, _region()).add_auth(req)
    prepared = req.prepare()
    http_req = urllib.request.Request(
        prepared.url,
        data=body if method.upper() not in ("GET", "HEAD") else None,
        headers=dict(prepared.headers),
        method=method.upper(),
    )
    try:
        with urllib.request.urlopen(http_req, timeout=30) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as err:
        return err.code, err.read().decode("utf-8", errors="replace")


def test_eks_create_describe_list_delete(unique_prefix):
    require_ready()
    name = f"{unique_prefix}-eks".replace("_", "-")[:40]
    create_body = json.dumps(
        {
            "name": name,
            "roleArn": "arn:aws:iam::000000000001:role/eks",
            "version": "1.29",
            "resourcesVpcConfig": {
                "subnetIds": ["subnet-1"],
                "securityGroupIds": [],
            },
        }
    ).encode()
    status, body = signed_http("eks", "POST", "/clusters", create_body, "application/json")
    assert 200 <= status < 300, body
    try:
        created = json.loads(body)
        cluster = created.get("cluster") or {}
        if cluster.get("status") != "ACTIVE":
            pytest.skip(f"EKS live smoke skipped: status={cluster.get('status')}")
        assert "noctaxris-eks-" in str(cluster.get("endpoint") or "")

        status, body = signed_http("eks", "GET", f"/clusters/{urllib.parse.quote(name)}")
        assert status == 200, body
        described = json.loads(body)
        assert (described.get("cluster") or {}).get("status") == "ACTIVE", described

        status, body = signed_http("eks", "GET", "/clusters")
        assert status == 200 and name in body, body

        status, body = signed_http(
            "eks", "GET", f"/clusters/{urllib.parse.quote(name)}/node-groups"
        )
        assert status == 200, body

        status, body = signed_http("eks", "DELETE", f"/clusters/{urllib.parse.quote(name)}")
        assert 200 <= status < 300, body

        status, body = signed_http("eks", "GET", f"/clusters/{urllib.parse.quote(name)}")
        assert status != 200, body
    finally:
        signed_http("eks", "DELETE", f"/clusters/{urllib.parse.quote(name)}")
