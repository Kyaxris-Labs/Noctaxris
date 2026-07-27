"""Lightsail + Auto Scaling + Elastic Beanstalk + Backup control-plane smokes."""

from __future__ import annotations

import json
import os
import urllib.parse
import urllib.request

import boto3
import pytest
from botocore.auth import SigV4Auth
from botocore.awsrequest import AWSRequest
from botocore.credentials import Credentials

from conftest import _access_key, _secret_key, endpoint, require_ready


def _region() -> str:
    return os.environ.get("AWS_DEFAULT_REGION", "us-east-1")


def _creds():
    return {
        "region_name": _region(),
        "endpoint_url": endpoint(),
        "aws_access_key_id": _access_key(),
        "aws_secret_access_key": _secret_key(),
    }


def signed_http(service: str, method: str, path: str, body: bytes | None = None, content_type: str = "") -> tuple[int, str]:
    url = endpoint() + path
    headers = {}
    if content_type:
        headers["Content-Type"] = content_type
    req = AWSRequest(method=method, url=url, data=body or b"", headers=headers)
    creds = Credentials(_access_key(), _secret_key())
    SigV4Auth(creds, service, _region()).add_auth(req)
    prepared = req.prepare()
    http_req = urllib.request.Request(prepared.url, data=prepared.body, headers=dict(prepared.headers), method=method)
    try:
        with urllib.request.urlopen(http_req, timeout=10) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except Exception as err:  # noqa: BLE001
        if hasattr(err, "code") and hasattr(err, "read"):
            return int(err.code), err.read().decode("utf-8", errors="replace")
        raise

@pytest.fixture
def lightsail_client():
    return boto3.client("lightsail", **_creds())


@pytest.fixture
def autoscaling_client():
    return boto3.client("autoscaling", **_creds())


@pytest.fixture
def elasticbeanstalk_client():
    return boto3.client("elasticbeanstalk", **_creds())


def test_lightsail_create_get_delete(lightsail_client, unique_prefix):
    require_ready()
    name = f"{unique_prefix}-ls"
    lightsail_client.create_instances(
        instanceNames=[name],
        availabilityZone="us-east-1a",
        blueprintId="ubuntu_22_04",
        bundleId="nano_3_0",
    )
    try:
        got = lightsail_client.get_instance(instanceName=name)
        assert got["instance"]["name"] == name
    finally:
        lightsail_client.delete_instance(instanceName=name)


def test_autoscaling_desired_capacity_reconciles_instances(autoscaling_client, unique_prefix):
    require_ready()
    lc = f"{unique_prefix}-lc"
    asg = f"{unique_prefix}-asg"
    autoscaling_client.create_launch_configuration(
        LaunchConfigurationName=lc,
        ImageId="ami-12345678",
        InstanceType="t3.micro",
    )
    try:
        autoscaling_client.create_auto_scaling_group(
            AutoScalingGroupName=asg,
            LaunchConfigurationName=lc,
            MinSize=1,
            MaxSize=3,
            DesiredCapacity=1,
            AvailabilityZones=["us-east-1a"],
        )
        autoscaling_client.set_desired_capacity(
            AutoScalingGroupName=asg,
            DesiredCapacity=2,
        )
        desc = autoscaling_client.describe_auto_scaling_groups(AutoScalingGroupNames=[asg])
        group = desc["AutoScalingGroups"][0]
        assert group["DesiredCapacity"] == 2
        members = group.get("Instances") or []
        assert len(members) == 2
        for m in members:
            assert str(m.get("InstanceId") or "").startswith("i-"), m
            ls = str(m.get("LifecycleState") or "")
            assert ls in ("Pending", "InService"), f"LifecycleState={ls!r} (Pending OK without engine)"
    finally:
        try:
            autoscaling_client.delete_auto_scaling_group(AutoScalingGroupName=asg, ForceDelete=True)
        except Exception:
            pass
        try:
            autoscaling_client.delete_launch_configuration(LaunchConfigurationName=lc)
        except Exception:
            pass


def test_beanstalk_ready_environment(elasticbeanstalk_client, unique_prefix):
    require_ready()
    app = f"{unique_prefix}-app"
    env = f"{unique_prefix}-env"
    elasticbeanstalk_client.create_application(ApplicationName=app)
    try:
        created = elasticbeanstalk_client.create_environment(
            ApplicationName=app,
            EnvironmentName=env,
        )
        assert created["Status"] == "Ready"
    finally:
        try:
            elasticbeanstalk_client.terminate_environment(EnvironmentName=env)
        except Exception:
            pass
        try:
            elasticbeanstalk_client.delete_application(ApplicationName=app, TerminateEnvByForce=True)
        except Exception:
            pass


def test_backup_vault_plan_job(unique_prefix):
    require_ready()
    vault = f"{unique_prefix}-vault".lower()
    status, body = signed_http("backup", "PUT", f"/backup-vaults/{urllib.parse.quote(vault)}", b"{}", "application/json")
    assert status == 200, body
    try:
        status, body = signed_http(
            "backup",
            "PUT",
            "/backup/plans/",
            json.dumps(
                {
                    "BackupPlan": {
                        "BackupPlanName": f"{unique_prefix}-plan",
                        "Rules": [
                            {
                                "RuleName": "daily",
                                "TargetBackupVaultName": vault,
                                "ScheduleExpression": "cron(0 5 ? * * *)",
                            }
                        ],
                    }
                }
            ).encode(),
            "application/json",
        )
        assert status == 200 and "BackupPlanId" in body, body
        plan = json.loads(body)
        plan_id = plan.get("BackupPlanId")

        status, body = signed_http(
            "backup",
            "PUT",
            "/backup-jobs",
            json.dumps(
                {
                    "BackupVaultName": vault,
                    "ResourceArn": "arn:aws:s3:::lab-bucket",
                    "IamRoleArn": "arn:aws:iam::000000000001:role/Backup",
                }
            ).encode(),
            "application/json",
        )
        assert status == 200 and "RecoveryPointArn" in body, body

        status, body = signed_http(
            "backup",
            "GET",
            f"/backup-vaults/{urllib.parse.quote(vault)}/recovery-points/",
        )
        assert status == 200 and "S3" in body, body
    finally:
        if "plan_id" in locals() and plan_id:
            signed_http("backup", "DELETE", f"/backup/plans/{urllib.parse.quote(plan_id)}")
        signed_http("backup", "DELETE", f"/backup-vaults/{urllib.parse.quote(vault)}")
