"""ELBv2 CreateRule path-pattern (control plane only; no /alb invoke)."""

from __future__ import annotations

from conftest import json_target


def test_elbv2_create_rule(unique_prefix):
    lb_name = f"{unique_prefix}-alb"[:32]
    tg_a = f"{unique_prefix}-tga"[:32]
    tg_b = f"{unique_prefix}-tgb"[:32]

    lb = json_target(
        "ElasticLoadBalancing_v2.CreateLoadBalancer",
        "elasticloadbalancing",
        {"Name": lb_name},
    )
    lb_arn = lb["LoadBalancers"][0]["LoadBalancerArn"]

    tg_a_out = json_target(
        "ElasticLoadBalancing_v2.CreateTargetGroup",
        "elasticloadbalancing",
        {"Name": tg_a, "TargetType": "lambda"},
    )
    tg_a_arn = tg_a_out["TargetGroups"][0]["TargetGroupArn"]

    tg_b_out = json_target(
        "ElasticLoadBalancing_v2.CreateTargetGroup",
        "elasticloadbalancing",
        {"Name": tg_b, "TargetType": "lambda"},
    )
    tg_b_arn = tg_b_out["TargetGroups"][0]["TargetGroupArn"]

    listener = json_target(
        "ElasticLoadBalancing_v2.CreateListener",
        "elasticloadbalancing",
        {
            "LoadBalancerArn": lb_arn,
            "Protocol": "HTTP",
            "Port": 80,
            "DefaultActions": [
                {"Type": "forward", "TargetGroupArn": tg_b_arn}
            ],
        },
    )
    listener_arn = listener["Listeners"][0]["ListenerArn"]

    rule = json_target(
        "ElasticLoadBalancing_v2.CreateRule",
        "elasticloadbalancing",
        {
            "ListenerArn": listener_arn,
            "Priority": 5,
            "Conditions": [
                {"Field": "path-pattern", "Values": ["/api*"]}
            ],
            "Actions": [
                {"Type": "forward", "TargetGroupArn": tg_a_arn}
            ],
        },
    )
    assert rule["Rules"][0]["RuleArn"]

    desc = json_target(
        "ElasticLoadBalancing_v2.DescribeRules",
        "elasticloadbalancing",
        {"ListenerArn": listener_arn},
    )
    assert "/api*" in str(desc)

    # Cleanup best-effort.
    try:
        json_target(
            "ElasticLoadBalancing_v2.DeleteRule",
            "elasticloadbalancing",
            {"RuleArn": rule["Rules"][0]["RuleArn"]},
        )
    except Exception:
        pass
    try:
        json_target(
            "ElasticLoadBalancing_v2.DeleteListener",
            "elasticloadbalancing",
            {"ListenerArn": listener_arn},
        )
    except Exception:
        pass
    try:
        json_target(
            "ElasticLoadBalancing_v2.DeleteTargetGroup",
            "elasticloadbalancing",
            {"TargetGroupArn": tg_a_arn},
        )
    except Exception:
        pass
    try:
        json_target(
            "ElasticLoadBalancing_v2.DeleteTargetGroup",
            "elasticloadbalancing",
            {"TargetGroupArn": tg_b_arn},
        )
    except Exception:
        pass
    try:
        json_target(
            "ElasticLoadBalancing_v2.DeleteLoadBalancer",
            "elasticloadbalancing",
            {"LoadBalancerArn": lb_arn},
        )
    except Exception:
        pass
