import io
import json
import os
import zipfile

import pytest

TRUST = (
    '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
    '"Principal":{"Service":"lambda.amazonaws.com"},'
    '"Action":"sts:AssumeRole"}]}'
)


def _minimal_python_zip() -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as zf:
        zf.writestr(
            "index.py",
            "def handler(event, context):\n    return {'ok': True}\n",
        )
    return buf.getvalue()


def test_lambda_nested_invoke(iam_client, lambda_client, unique_prefix):
    if os.environ.get("NOCTAXRIS_NESTED") != "1":
        pytest.skip(
            "nested Lambda Invoke — set NOCTAXRIS_NESTED=1 "
            "(Compose noctaxris-engine healthy)"
        )

    role_name = f"{unique_prefix}-lambda-role"
    fn_name = f"{unique_prefix}-invoke-fn"

    role_out = iam_client.create_role(
        RoleName=role_name,
        AssumeRolePolicyDocument=TRUST,
    )
    role_arn = role_out["Role"]["Arn"]
    assert role_arn

    try:
        lambda_client.create_function(
            FunctionName=fn_name,
            Runtime="python3.12",
            Role=role_arn,
            Handler="index.handler",
            Code={"ZipFile": _minimal_python_zip()},
        )
        try:
            inv = lambda_client.invoke(FunctionName=fn_name, Payload=b"{}")
            assert not inv.get("FunctionError"), inv
            raw = inv["Payload"].read()
            body = json.loads(raw)
            assert body.get("ok") is True, body
        finally:
            try:
                lambda_client.delete_function(FunctionName=fn_name)
            except Exception:
                pass
    finally:
        try:
            iam_client.delete_role(RoleName=role_name)
        except Exception:
            pass
