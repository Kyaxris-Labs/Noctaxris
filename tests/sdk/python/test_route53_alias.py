"""Route53 Alias A to CloudFront DomainName (lab JSON targets)."""

from __future__ import annotations

from conftest import json_target


def test_route53_alias_to_cloudfront(s3_client, unique_prefix):
    bucket = f"{unique_prefix}-r53-cf".lower()
    zone_name = f"{unique_prefix.replace('_', '-')}.example.com"
    s3_client.create_bucket(Bucket=bucket)
    dist_id = None
    zone_id = None
    try:
        cf = json_target(
            "CloudFront_2016_01_28.CreateDistribution",
            "cloudfront",
            {
                "DistributionConfig": {
                    "CallerReference": f"{unique_prefix}-r53-cf",
                    "Comment": "r53",
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
        dist_id = cf["Distribution"]["Id"]
        domain = cf["Distribution"]["DomainName"]

        zone = json_target(
            "AWSRoute53.CreateHostedZone",
            "route53",
            {
                "Name": zone_name,
                "CallerReference": f"{unique_prefix}-hz",
            },
        )
        zone_id = zone["HostedZone"]["Id"].removeprefix("/hostedzone/")

        json_target(
            "AWSRoute53.ChangeResourceRecordSets",
            "route53",
            {
                "HostedZoneId": zone_id,
                "ChangeBatch": {
                    "Changes": [
                        {
                            "Action": "CREATE",
                            "ResourceRecordSet": {
                                "Name": f"www.{zone_name}",
                                "Type": "A",
                                "AliasTarget": {
                                    "DNSName": domain,
                                    "HostedZoneId": "Z2FDTNDATAQYW2",
                                    "EvaluateTargetHealth": False,
                                },
                            },
                        }
                    ]
                },
            },
        )

        listed = json_target(
            "AWSRoute53.ListResourceRecordSets",
            "route53",
            {"HostedZoneId": zone_id},
        )
        body = str(listed).lower()
        assert "aliastarget" in body
        assert domain.lower() in body
    finally:
        if zone_id:
            try:
                json_target(
                    "AWSRoute53.DeleteHostedZone",
                    "route53",
                    {"Id": zone_id},
                )
            except Exception:
                pass
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
            s3_client.delete_bucket(Bucket=bucket)
        except Exception:
            pass
