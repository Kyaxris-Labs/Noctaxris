"""API Gateway REST API MOCK execute smoke."""

from __future__ import annotations

import urllib.request

import pytest
from conftest import endpoint


def test_rest_api_mock_execute(apigateway_client, unique_prefix):
    apigw = apigateway_client
    name = f"{unique_prefix}-rest"
    api = apigw.create_rest_api(name=name)
    api_id = api["id"]
    try:
        resources = apigw.get_resources(restApiId=api_id)
        root = next(i for i in resources["items"] if i.get("path") == "/")
        child = apigw.create_resource(
            restApiId=api_id, parentId=root["id"], pathPart="hello"
        )
        apigw.put_method(
            restApiId=api_id,
            resourceId=child["id"],
            httpMethod="GET",
            authorizationType="NONE",
        )
        apigw.put_integration(
            restApiId=api_id,
            resourceId=child["id"],
            httpMethod="GET",
            type="MOCK",
            requestTemplates={"application/json": '{"message":"sdk-mock-ok"}'},
        )
        apigw.create_deployment(restApiId=api_id, stageName="dev")
        url = f"{endpoint()}/restapis/{api_id}/dev/_user_request_/hello"
        with urllib.request.urlopen(url, timeout=30) as resp:
            body = resp.read().decode("utf-8")
            assert resp.status == 200
            assert "sdk-mock-ok" in body
    except Exception as exc:
        if "NOCTAXRIS_ALLOW_OPEN_DATA_PLANE" in str(exc):
            pytest.skip(
                "REST MOCK soft-skip: set NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1 "
                "(compose.lab-open.yaml) for AuthType NONE on non-loopback listen"
            )
        raise
    finally:
        try:
            apigw.delete_rest_api(restApiId=api_id)
        except Exception:
            pass
