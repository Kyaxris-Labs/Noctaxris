"""ELBv2 Network Load Balancer Type=network create/describe/cleanup."""

from __future__ import annotations

from conftest import json_target


def test_elbv2_network_load_balancer(unique_prefix):
    lb_name = f"{unique_prefix}-nlb"[:32]

    lb = json_target(
        "ElasticLoadBalancing_v2.CreateLoadBalancer",
        "elasticloadbalancing",
        {"Name": lb_name, "Type": "network"},
    )
    lb_arn = lb["LoadBalancers"][0]["LoadBalancerArn"]
    assert lb["LoadBalancers"][0]["Type"] == "network"
    assert "loadbalancer/net/" in lb_arn

    try:
        desc = json_target(
            "ElasticLoadBalancing_v2.DescribeLoadBalancers",
            "elasticloadbalancing",
            {"LoadBalancerArns": [lb_arn]},
        )
        assert desc["LoadBalancers"][0]["Type"] == "network"
    finally:
        try:
            json_target(
                "ElasticLoadBalancing_v2.DeleteLoadBalancer",
                "elasticloadbalancing",
                {"LoadBalancerArn": lb_arn},
            )
        except Exception:
            pass
