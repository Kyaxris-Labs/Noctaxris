import io
import json
import urllib.error
import urllib.request
import zipfile

import pytest

from conftest import endpoint

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


def test_websocket_api_create_routes_soft_skip_connect(
    iam_client, lambda_client, apigatewayv2_client, unique_prefix
):
    role_name = f"{unique_prefix}-ws-role"
    role_out = iam_client.create_role(
        RoleName=role_name,
        AssumeRolePolicyDocument=TRUST,
    )
    role_arn = role_out["Role"]["Arn"]

    try:
        fn_name = f"{unique_prefix}-ws-fn"
        fn_out = lambda_client.create_function(
            FunctionName=fn_name,
            Runtime="python3.12",
            Role=role_arn,
            Handler="index.handler",
            Code={"ZipFile": _minimal_python_zip()},
        )
        lambda_arn = fn_out["FunctionArn"]
        lambda_client.add_permission(
            FunctionName=fn_name,
            StatementId=f"{unique_prefix}-ws-perm",
            Action="lambda:InvokeFunction",
            Principal="apigateway.amazonaws.com",
        )

        try:
            api_out = apigatewayv2_client.create_api(
                Name=f"{unique_prefix}-ws-api",
                ProtocolType="WEBSOCKET",
            )
            assert api_out["ProtocolType"] == "WEBSOCKET"
            api_id = api_out["ApiId"]

            try:
                int_out = apigatewayv2_client.create_integration(
                    ApiId=api_id,
                    IntegrationType="AWS_PROXY",
                    IntegrationUri=lambda_arn,
                )
                integration_id = int_out["IntegrationId"]
                for rk in ("$connect", "$disconnect", "$default"):
                    apigatewayv2_client.create_route(
                        ApiId=api_id,
                        RouteKey=rk,
                        Target=f"integrations/{integration_id}",
                        AuthorizationType="NONE",
                    )
                apigatewayv2_client.create_stage(
                    ApiId=api_id,
                    StageName="$default",
                    AutoDeploy=True,
                )

                req = urllib.request.Request(
                    f"{endpoint()}/ws-api/{api_id}/$default/$connect",
                    method="POST",
                )
                try:
                    with urllib.request.urlopen(req, timeout=30) as resp:
                        body = resp.read().decode()
                        assert resp.status == 200
                        data = json.loads(body)
                        assert data.get("connectionId")
                except urllib.error.HTTPError as e:
                    raw = e.read().decode(errors="replace")
                    if e.code == 503 and (
                        "compute unavailable" in raw
                        or __import__("os").environ.get("NOCTAXRIS_NESTED") != "1"
                    ):
                        pytest.skip(
                            "WebSocket $connect soft-skip: nested Lambda unavailable "
                            "(set NOCTAXRIS_NESTED=1)"
                        )
                    raise
            finally:
                apigatewayv2_client.delete_api(ApiId=api_id)
        finally:
            lambda_client.delete_function(FunctionName=fn_name)
    finally:
        iam_client.delete_role(RoleName=role_name)
