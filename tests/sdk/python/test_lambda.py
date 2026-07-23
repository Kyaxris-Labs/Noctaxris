import io
import zipfile

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


def test_lambda_create_get_delete(iam_client, lambda_client, unique_prefix):
    role_name = f"{unique_prefix}-lambda-role"
    fn_name = f"{unique_prefix}-fn"

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
            got = lambda_client.get_function(FunctionName=fn_name)
            assert got["Configuration"]["FunctionName"] == fn_name

            names = [
                f["FunctionName"]
                for f in lambda_client.list_functions().get("Functions", [])
            ]
            assert fn_name in names, f"ListFunctions missing {fn_name}"

            # Invoke requires nested DinD; CRUD is enough for this suite.
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
