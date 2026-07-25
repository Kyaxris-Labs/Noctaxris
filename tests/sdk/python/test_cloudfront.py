"""CloudFront lab CreateDistribution + optional fake-edge GET."""

from __future__ import annotations

from conftest import endpoint, json_target, signed_request


def test_cloudfront_deployed_domain_and_edge(s3_client, unique_prefix):
    bucket = f"{unique_prefix}-cf-origin".lower()
    key = "hello.txt"
    body = b"noctaxris-cf-edge"

    s3_client.create_bucket(Bucket=bucket)
    s3_client.put_object(Bucket=bucket, Key=key, Body=body)
    dist_id = None
    try:
        created = json_target(
            "CloudFront_2016_01_28.CreateDistribution",
            "cloudfront",
            {
                "DistributionConfig": {
                    "CallerReference": f"{unique_prefix}-cf-ref",
                    "Comment": "py sdk",
                    "Enabled": True,
                    "Origins": {
                        "Items": [
                            {
                                "Id": "o1",
                                "DomainName": bucket,
                                "OriginType": "s3",
                            }
                        ]
                    },
                }
            },
        )
        dist = created["Distribution"]
        dist_id = dist["Id"]
        assert dist.get("Status") == "Deployed"
        domain = dist.get("DomainName") or ""
        assert "cloudfront.noctaxris.local" in domain

        got = json_target(
            "CloudFront_2016_01_28.GetDistribution",
            "cloudfront",
            {"Id": dist_id},
        )
        assert got["Distribution"]["Id"] == dist_id

        status, edge_body, _ = signed_request(
            "GET",
            f"{endpoint()}/cloudfront/{dist_id}/{key}",
            service="cloudfront",
        )
        assert status == 200, edge_body
        assert edge_body == body
    finally:
        if dist_id:
            try:
                json_target(
                    "CloudFront_2016_01_28.DeleteDistribution",
                    "cloudfront",
                    {"Id": dist_id},
                )
            except Exception:
                pass
        try:
            s3_client.delete_object(Bucket=bucket, Key=key)
        except Exception:
            pass
        try:
            s3_client.delete_bucket(Bucket=bucket)
        except Exception:
            pass
