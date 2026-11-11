import json
import os
import urllib.error
import urllib.request

import boto3
import pytest
from botocore.auth import SigV4Auth
from botocore.awsrequest import AWSRequest
from botocore.client import Config
from botocore.credentials import Credentials
from conftest import endpoint


SEED_PATH = "/_noctaxris/lab/accounts:seedStandalone"


class _RawPathSigV4(SigV4Auth):
    """Sign the seed path with the colon left as-is."""

    def _normalize_url_path(self, path):
        if not path:
            return "/"
        return path


def _access_key():
    return os.environ.get("AWS_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")


def _secret_key():
    return os.environ.get(
        "AWS_SECRET_ACCESS_KEY",
        "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
    )


def _region():
    return os.environ.get("AWS_DEFAULT_REGION", "us-east-1")


def _reachability_message():
    ep = endpoint()
    url = f"{ep}/_noctaxris/ready"
    try:
        with urllib.request.urlopen(url, timeout=2) as resp:
            if resp.status != 200:
                return f"Noctaxris not reachable at {ep}: status {resp.status}"
    except (urllib.error.URLError, TimeoutError, OSError) as err:
        return f"Noctaxris not reachable at {ep}: {err}"
    return None


# The shared ready fixture fails when the API is down unless
# NOCTAXRIS_SKIP_IF_DOWN is set. This module skips instead.
@pytest.fixture(scope="session", autouse=True)
def _api_ready():
    message = _reachability_message()
    if message:
        pytest.skip(message)


def _seed_failure(status, raw):
    text = raw.decode("utf-8", "replace")
    if "secretAccessKey" in text:
        return f"seed status {status}"
    return f"seed status {status} body {text[:400]}"


def _seed(name):
    body = json.dumps({"name": name}, separators=(",", ":")).encode("utf-8")
    url = endpoint() + SEED_PATH
    req = AWSRequest(
        method="POST",
        url=url,
        data=body,
        headers={"Content-Type": "application/json"},
    )
    _RawPathSigV4(Credentials(_access_key(), _secret_key()), "iam", _region()).add_auth(
        req
    )
    prepared = req.prepare()
    http_req = urllib.request.Request(
        prepared.url,
        data=body,
        headers=dict(prepared.headers),
        method="POST",
    )
    try:
        with urllib.request.urlopen(http_req, timeout=30) as resp:
            status, raw = resp.status, resp.read()
    except urllib.error.HTTPError as err:
        status, raw = err.code, err.read()
    if status == 404:
        return status, None
    if status != 200:
        raise AssertionError(_seed_failure(status, raw))
    try:
        parsed = json.loads(raw.decode("utf-8"))
    except json.JSONDecodeError as err:
        raise AssertionError("seed response was not JSON") from err
    return status, parsed


def _s3(access_key_id, secret_access_key):
    return boto3.client(
        "s3",
        aws_access_key_id=access_key_id,
        aws_secret_access_key=secret_access_key,
        region_name=_region(),
        endpoint_url=endpoint(),
        config=Config(s3={"addressing_style": "path"}),
    )


def _bucket_names(client):
    buckets = client.list_buckets().get("Buckets") or []
    return [item["Name"] for item in buckets]


def test_standalone_account_s3_isolation():
    down = _reachability_message()
    if down:
        pytest.skip(down)

    name = f"sdk-py-{os.urandom(8).hex()}"
    home_bucket = name.lower()
    lab_bucket = f"{name}-lab".lower()
    object_key = "note.txt"
    payload = b"lab-root-object"

    status, seeded = _seed(name)
    if status == 404:
        pytest.skip("NOCTAXRIS_LAB_STANDALONE is off")
    assert seeded is not None

    access_key_id = seeded.get("accessKeyId")
    secret_access_key = seeded.get("secretAccessKey")
    assert isinstance(access_key_id, str) and access_key_id, (
        "seed response missing accessKeyId"
    )
    assert isinstance(secret_access_key, str) and secret_access_key, (
        "seed response missing secret"
    )

    home = _s3(access_key_id, secret_access_key)
    root = _s3(_access_key(), _secret_key())
    created_home = False
    created_lab = False
    try:
        home.create_bucket(Bucket=home_bucket)
        created_home = True

        assert home_bucket not in _bucket_names(root), (
            f"root ListBuckets includes {home_bucket}"
        )
        assert home_bucket in _bucket_names(home), (
            f"home ListBuckets missing {home_bucket}"
        )

        root.create_bucket(Bucket=lab_bucket)
        created_lab = True
        root.put_object(Bucket=lab_bucket, Key=object_key, Body=payload)

        returned_object = False
        try:
            got = home.get_object(Bucket=lab_bucket, Key=object_key)["Body"].read()
            returned_object = got == payload
        except Exception:
            returned_object = False
        assert returned_object is False, "home GetObject returned the root object"

        again_status, again = _seed(name)
        assert again_status == 200
        assert again is not None
        assert again.get("accessKeyId") == access_key_id
    finally:
        if created_lab:
            try:
                root.delete_object(Bucket=lab_bucket, Key=object_key)
            except Exception:
                pass
            try:
                root.delete_bucket(Bucket=lab_bucket)
            except Exception:
                pass
        if created_home:
            try:
                home.delete_bucket(Bucket=home_bucket)
            except Exception:
                pass
