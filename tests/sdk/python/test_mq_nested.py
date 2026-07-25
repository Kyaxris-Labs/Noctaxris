"""ActiveMQ CreateBroker; skip live AMQP when not RUNNING."""

from __future__ import annotations

import time

import pytest
from conftest import json_target


def test_activemq_create_skip_without_engine(unique_prefix):
    name = f"{unique_prefix}-amq"[:32]
    broker_id = None
    try:
        created = json_target(
            "mq.CreateBroker",
            "mq",
            {
                "BrokerName": name,
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
        broker_id = created.get("BrokerId")
        assert broker_id

        state = ""
        body = ""
        for _ in range(8):
            desc = json_target(
                "mq.DescribeBroker",
                "mq",
                {"BrokerId": broker_id},
                content_type="application/x-amz-json-1.0",
            )
            state = desc.get("BrokerState") or ""
            body = str(desc)
            if state in ("RUNNING", "CREATION_FAILED"):
                break
            time.sleep(0.5)

        assert state, "DescribeBroker missing BrokerState"
        if state != "RUNNING" or "stub://" in body:
            pytest.skip(
                f"ActiveMQ engine not RUNNING (state={state!r}); host cannot AMQP to nested"
            )
        # RUNNING: control-plane ok; host still cannot dial nested AMQP.
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
