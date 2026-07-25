"""Lambda MQ ESM; skip CreateEventSourceMapping when broker not RUNNING."""

from __future__ import annotations

import time

import pytest
from conftest import LAMBDA_TRUST, json_target, minimal_python_zip


def test_lambda_mq_esm_skip_without_engine(
    iam_client, lambda_client, account_id, unique_prefix
):
    broker_name = f"{unique_prefix}-mq"[:32]
    fn_name = f"{unique_prefix}-mq-fn"
    role_name = f"{unique_prefix}-mq-lam"
    broker_id = None

    role = iam_client.create_role(
        RoleName=role_name,
        AssumeRolePolicyDocument=LAMBDA_TRUST,
    )
    role_arn = role["Role"]["Arn"]
    iam_client.put_role_policy(
        RoleName=role_name,
        PolicyName="mq",
        PolicyDocument=(
            '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
            '"Action":["mq:DescribeBroker"],"Resource":"*"}]}'
        ),
    )

    try:
        created = json_target(
            "mq.CreateBroker",
            "mq",
            {
                "BrokerName": broker_name,
                "EngineType": "ACTIVEMQ",
                "EngineVersion": "5.18",
                "HostInstanceType": "mq.t3.micro",
                "DeploymentMode": "SINGLE_INSTANCE",
                "PubliclyAccessible": False,
                "Users": [
                    {"Username": "lab", "Password": "lab-password-1"}
                ],
            },
            content_type="application/x-amz-json-1.0",
        )
        broker_id = created["BrokerId"]

        state = ""
        for _ in range(8):
            desc = json_target(
                "mq.DescribeBroker",
                "mq",
                {"BrokerId": broker_id},
                content_type="application/x-amz-json-1.0",
            )
            state = desc.get("BrokerState") or ""
            if state in ("RUNNING", "CREATION_FAILED"):
                break
            time.sleep(0.5)

        if state != "RUNNING":
            pytest.skip(f"MQ broker not RUNNING (state={state!r}); skip ESM")

        lambda_client.create_function(
            FunctionName=fn_name,
            Runtime="python3.12",
            Role=role_arn,
            Handler="index.handler",
            Code={"ZipFile": minimal_python_zip()},
        )
        broker_arn = (
            f"arn:aws:mq:us-east-1:{account_id}:broker:{broker_name}:{broker_id}"
        )
        try:
            esm = lambda_client.create_event_source_mapping(
                FunctionName=fn_name,
                EventSourceArn=broker_arn,
                BatchSize=5,
                Enabled=True,
            )
            assert esm.get("UUID")
            # Invoke poll needs nested; control-plane mapping is enough.
        finally:
            try:
                mappings = lambda_client.list_event_source_mappings(
                    FunctionName=fn_name
                )
                for m in mappings.get("EventSourceMappings") or []:
                    lambda_client.delete_event_source_mapping(UUID=m["UUID"])
            except Exception:
                pass
            try:
                lambda_client.delete_function(FunctionName=fn_name)
            except Exception:
                pass
    finally:
        if broker_id:
            try:
                json_target(
                    "mq.DeleteBroker",
                    "mq",
                    {"BrokerId": broker_id},
                    content_type="application/x-amz-json-1.0",
                )
            except Exception:
                pass
        try:
            iam_client.delete_role_policy(RoleName=role_name, PolicyName="mq")
        except Exception:
            pass
        try:
            iam_client.delete_role(RoleName=role_name)
        except Exception:
            pass
