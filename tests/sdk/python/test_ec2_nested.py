"""EC2 Run → Stop → Start → Terminate (pending OK without engine)."""

from __future__ import annotations

import boto3
from conftest import _creds, require_ready


def ec2_client():
    return boto3.client("ec2", **_creds())


def test_ec2_run_stop_start_terminate_pending_ok():
    require_ready()
    c = ec2_client()
    run = c.run_instances(
        ImageId="ami-alpine",
        InstanceType="t3.micro",
        MinCount=1,
        MaxCount=1,
    )
    inst = (run.get("Instances") or [None])[0]
    assert inst and inst.get("InstanceId")
    iid = inst["InstanceId"]
    assert iid.startswith("i-")
    try:
        desc = c.describe_instances(InstanceIds=[iid])
        reservations = desc.get("Reservations") or []
        assert reservations and reservations[0].get("Instances")
        state = (reservations[0]["Instances"][0].get("State") or {}).get("Name", "")
        assert state in ("pending", "running"), f"after Run want pending|running got {state!r}"

        stop = c.stop_instances(InstanceIds=[iid])
        assert len(stop.get("StoppingInstances") or []) >= 1

        start = c.start_instances(InstanceIds=[iid])
        starting = start.get("StartingInstances") or []
        assert len(starting) >= 1
        started = (starting[0].get("CurrentState") or {}).get("Name", "")
        assert started in (
            "pending",
            "running",
        ), f"after Start want pending|running got {started!r}"

        term = c.terminate_instances(InstanceIds=[iid])
        assert len(term.get("TerminatingInstances") or []) >= 1
    finally:
        try:
            c.terminate_instances(InstanceIds=[iid])
        except Exception:
            pass
