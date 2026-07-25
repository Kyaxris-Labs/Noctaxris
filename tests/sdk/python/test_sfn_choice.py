"""Step Functions Choice execution SUCCEEDED."""

from __future__ import annotations

import json


def test_sfn_choice_execution_succeeded(stepfunctions_client, unique_prefix):
    name = f"{unique_prefix}-choice"
    definition = {
        "StartAt": "Pick",
        "States": {
            "Pick": {
                "Type": "Choice",
                "Choices": [
                    {
                        "Variable": "$.color",
                        "StringEquals": "red",
                        "Next": "Red",
                    }
                ],
                "Default": "Other",
            },
            "Red": {"Type": "Pass", "Result": {"branch": "red"}, "End": True},
            "Other": {"Type": "Pass", "Result": {"branch": "other"}, "End": True},
        },
    }
    sm_arn = None
    try:
        created = stepfunctions_client.create_state_machine(
            name=name,
            definition=json.dumps(definition),
        )
        sm_arn = created["stateMachineArn"]

        started = stepfunctions_client.start_execution(
            stateMachineArn=sm_arn,
            name=f"{unique_prefix}-exec",
            input=json.dumps({"color": "red"}),
        )
        exec_arn = started["executionArn"]
        desc = stepfunctions_client.describe_execution(executionArn=exec_arn)
        assert desc["status"] == "SUCCEEDED"
        out = json.loads(desc.get("output") or "{}")
        assert out.get("branch") == "red" or "red" in str(desc.get("output"))
    finally:
        if sm_arn:
            try:
                stepfunctions_client.delete_state_machine(stateMachineArn=sm_arn)
            except Exception:
                pass
