import io
import urllib.error
import urllib.request
import zipfile

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


def test_http_api_cors_preflight_and_update(
    iam_client, lambda_client, apigatewayv2_client, unique_prefix
):
    role_name = f"{unique_prefix}-cors-role"
    role_out = iam_client.create_role(
        RoleName=role_name,
        AssumeRolePolicyDocument=TRUST,
    )
    role_arn = role_out["Role"]["Arn"]
    assert role_arn

    try:
        fn_name = f"{unique_prefix}-cors-fn"
        fn_out = lambda_client.create_function(
            FunctionName=fn_name,
            Runtime="python3.12",
            Role=role_arn,
            Handler="index.handler",
            Code={"ZipFile": _minimal_python_zip()},
        )
        lambda_arn = fn_out["FunctionArn"]
        assert lambda_arn

        try:
            api_out = apigatewayv2_client.create_api(
                Name=f"{unique_prefix}-cors-api",
                ProtocolType="HTTP",
                CorsConfiguration={
                    "AllowOrigins": ["https://lab.example"],
                    "AllowMethods": ["GET", "OPTIONS"],
                    "AllowHeaders": ["authorization", "content-type"],
                    "MaxAge": 600,
                },
            )
            api_id = api_out["ApiId"]
            assert api_id
            assert api_out.get("CorsConfiguration")

            try:
                int_out = apigatewayv2_client.create_integration(
                    ApiId=api_id,
                    IntegrationType="AWS_PROXY",
                    IntegrationUri=lambda_arn,
                )
                integration_id = int_out["IntegrationId"]
                assert integration_id

                apigatewayv2_client.create_route(
                    ApiId=api_id,
                    RouteKey="GET /hello",
                    Target=f"integrations/{integration_id}",
                    AuthorizationType="NONE",
                )
                apigatewayv2_client.create_stage(
                    ApiId=api_id,
                    StageName="$default",
                )

                hello_url = f"{endpoint()}/http-api/{api_id}/$default/hello"
                opt_req = urllib.request.Request(
                    hello_url,
                    method="OPTIONS",
                    headers={
                        "Origin": "https://lab.example",
                        "Access-Control-Request-Method": "GET",
                        "Access-Control-Request-Headers": "authorization",
                    },
                )
                with urllib.request.urlopen(opt_req, timeout=30) as opt_resp:
                    assert opt_resp.status == 204
                    assert (
                        opt_resp.headers.get("Access-Control-Allow-Origin")
                        == "https://lab.example"
                    )
                    assert "GET" in (
                        opt_resp.headers.get("Access-Control-Allow-Methods") or ""
                    )
                    assert opt_resp.headers.get("Access-Control-Max-Age") == "600"

                deny_req = urllib.request.Request(
                    hello_url,
                    method="OPTIONS",
                    headers={
                        "Origin": "https://evil.example",
                        "Access-Control-Request-Method": "GET",
                    },
                )
                try:
                    with urllib.request.urlopen(deny_req, timeout=30) as deny_resp:
                        status = deny_resp.status
                except urllib.error.HTTPError as err:
                    status = err.code
                assert status == 403, f"foreign origin preflight status={status}"

                apigatewayv2_client.update_api(
                    ApiId=api_id,
                    CorsConfiguration={
                        "AllowOrigins": ["https://other.example"],
                        "AllowMethods": ["GET", "POST", "OPTIONS"],
                    },
                )

                opt2_req = urllib.request.Request(
                    hello_url,
                    method="OPTIONS",
                    headers={
                        "Origin": "https://other.example",
                        "Access-Control-Request-Method": "POST",
                    },
                )
                with urllib.request.urlopen(opt2_req, timeout=30) as opt2_resp:
                    assert opt2_resp.status == 204
                    assert (
                        opt2_resp.headers.get("Access-Control-Allow-Origin")
                        == "https://other.example"
                    )
            finally:
                try:
                    apigatewayv2_client.delete_api(ApiId=api_id)
                except Exception:
                    pass
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
