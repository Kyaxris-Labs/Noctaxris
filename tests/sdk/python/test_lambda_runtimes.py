import io
import zipfile

import pytest
from botocore.exceptions import ClientError

TRUST = (
    '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
    '"Principal":{"Service":"lambda.amazonaws.com"},'
    '"Action":"sts:AssumeRole"}]}'
)


def _zip_bytes(name: str, body: str) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as zf:
        zf.writestr(name, body)
    return buf.getvalue()


def _python_zip() -> bytes:
    return _zip_bytes(
        "index.py",
        "def handler(event, context):\n    return {'ok': True}\n",
    )


def _node_zip() -> bytes:
    return _zip_bytes(
        "index.js",
        "exports.handler = async () => ({ ok: true });\n",
    )


def _java_zip() -> bytes:
    return _zip_bytes(
        "example/Handler.java",
        """package example;
public class Handler {
  public static String handleRequest(String in) { return "{\\"ok\\":true}"; }
}
""",
    )


CASES = [
    ("py311", "python3.11", "index.handler", _python_zip),
    ("py312", "python3.12", "index.handler", _python_zip),
    ("py313", "python3.13", "index.handler", _python_zip),
    ("py314", "python3.14", "index.handler", _python_zip),
    ("node20", "nodejs20.x", "index.handler", _node_zip),
    ("node22", "nodejs22.x", "index.handler", _node_zip),
    ("node24", "nodejs24.x", "index.handler", _node_zip),
    ("java21", "java21", "example.Handler::handleRequest", _java_zip),
    ("java25", "java25", "example.Handler::handleRequest", _java_zip),
]


@pytest.mark.parametrize("suffix,runtime,handler,zip_fn", CASES)
def test_lambda_create_get_delete_runtime(
    iam_client, lambda_client, unique_prefix, suffix, runtime, handler, zip_fn
):
    role_name = f"{unique_prefix}-lambda-rt-{suffix}"
    fn_name = f"{unique_prefix}-{suffix}"

    role_out = iam_client.create_role(
        RoleName=role_name,
        AssumeRolePolicyDocument=TRUST,
    )
    role_arn = role_out["Role"]["Arn"]
    try:
        lambda_client.create_function(
            FunctionName=fn_name,
            Runtime=runtime,
            Role=role_arn,
            Handler=handler,
            Code={"ZipFile": zip_fn()},
        )
        try:
            got = lambda_client.get_function(FunctionName=fn_name)
            assert got["Configuration"]["Runtime"] == runtime
            assert got["Configuration"]["Handler"] == handler
            lambda_client.delete_function(FunctionName=fn_name)
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


def test_lambda_create_rejects_unsupported_runtime(
    iam_client, lambda_client, unique_prefix
):
    role_name = f"{unique_prefix}-lambda-bad-rt"
    role_out = iam_client.create_role(
        RoleName=role_name,
        AssumeRolePolicyDocument=TRUST,
    )
    role_arn = role_out["Role"]["Arn"]
    try:
        with pytest.raises(ClientError):
            lambda_client.create_function(
                FunctionName=f"{unique_prefix}-bad",
                Runtime="ruby3.4",
                Role=role_arn,
                Handler="index.handler",
                Code={"ZipFile": _python_zip()},
            )
    finally:
        try:
            iam_client.delete_role(RoleName=role_name)
        except Exception:
            pass
