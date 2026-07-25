"""OpenSearch CreateDomain; skip query facade when engine not Active."""

from __future__ import annotations

import json
import time

import pytest
from conftest import endpoint, json_target, signed_request


def _describe(domain: str) -> dict:
    return json_target(
        "AmazonOpenSearchService.DescribeDomain",
        "es",
        {"DomainName": domain},
        content_type="application/x-amz-json-1.0",
    )


def test_opensearch_create_skip_query_without_engine(unique_prefix):
    domain = f"{unique_prefix}-os".replace("_", "-")[:28]
    try:
        json_target(
            "AmazonOpenSearchService.CreateDomain",
            "es",
            {"DomainName": domain, "EngineVersion": "OpenSearch_2.11"},
            content_type="application/x-amz-json-1.0",
        )

        status = ""
        for _ in range(8):
            desc = _describe(domain)
            st = desc.get("DomainStatus") or {}
            if isinstance(st, dict):
                status = st.get("DomainStatus") or (
                    "Active" if st.get("Created") else ""
                )
                endpoint_s = str(st.get("Endpoint") or "")
            else:
                status = str(st)
                endpoint_s = ""
            if status in ("Active", "CreateFailed") or "stub://" in endpoint_s:
                break
            time.sleep(0.5)

        desc = _describe(domain)
        st = desc.get("DomainStatus") or {}
        if isinstance(st, dict):
            status = st.get("DomainStatus") or (
                "Active" if st.get("Created") else "CreateFailed"
            )
            endpoint_s = str(st.get("Endpoint") or "")
            created = bool(st.get("Created"))
        else:
            status = str(st)
            endpoint_s = ""
            created = status == "Active"

        if status != "Active" or not created or endpoint_s.startswith("stub://"):
            pytest.skip(
                f"OpenSearch engine not Active (status={status!r} endpoint={endpoint_s!r})"
            )

        # Nested Active: exercise lab query facade.
        put_status, _, _ = signed_request(
            "PUT",
            f"{endpoint()}/opensearch/{domain}/lab/books/_doc/1",
            service="es",
            body=json.dumps({"title": "hello"}).encode("utf-8"),
            content_type="application/json",
        )
        assert put_status in (200, 201), put_status

        search_status, search_body, _ = signed_request(
            "POST",
            f"{endpoint()}/opensearch/{domain}/lab/books/_search",
            service="es",
            body=json.dumps(
                {"query": {"match": {"title": "hello"}}, "size": 5}
            ).encode("utf-8"),
            content_type="application/json",
        )
        assert search_status == 200, search_body
    finally:
        try:
            json_target(
                "AmazonOpenSearchService.DeleteDomain",
                "es",
                {"DomainName": domain},
                content_type="application/x-amz-json-1.0",
            )
        except Exception:
            pass
