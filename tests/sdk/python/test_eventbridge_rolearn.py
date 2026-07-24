import json
import time


EB_SOURCE = "noctaxris.sdk.eb.rolearn"


def _queue_arn(sqs_client, queue_url: str) -> str:
    attrs = sqs_client.get_queue_attributes(
        QueueUrl=queue_url,
        AttributeNames=["QueueArn"],
    )
    arn = attrs["Attributes"]["QueueArn"]
    assert arn
    return arn


def _drain_queue(sqs_client, queue_url: str) -> None:
    deadline = time.time() + 3
    while time.time() < deadline:
        recv = sqs_client.receive_message(
            QueueUrl=queue_url,
            MaxNumberOfMessages=10,
            WaitTimeSeconds=1,
            VisibilityTimeout=0,
        )
        msgs = recv.get("Messages") or []
        if not msgs:
            return
        for m in msgs:
            rh = m.get("ReceiptHandle")
            if not rh:
                continue
            sqs_client.delete_message(QueueUrl=queue_url, ReceiptHandle=rh)


def _receive_one_body(sqs_client, queue_url: str, timeout_s: float) -> str:
    deadline = time.time() + timeout_s
    while time.time() < deadline:
        recv = sqs_client.receive_message(
            QueueUrl=queue_url,
            MaxNumberOfMessages=1,
            WaitTimeSeconds=1,
        )
        msgs = recv.get("Messages") or []
        if not msgs:
            continue
        body = msgs[0]["Body"]
        rh = msgs[0].get("ReceiptHandle")
        if rh:
            sqs_client.delete_message(QueueUrl=queue_url, ReceiptHandle=rh)
        return body
    raise AssertionError(f"ReceiveMessage timed out on {queue_url}")


def _receive_optional_body(sqs_client, queue_url: str, timeout_s: float) -> str:
    deadline = time.time() + timeout_s
    while time.time() < deadline:
        recv = sqs_client.receive_message(
            QueueUrl=queue_url,
            MaxNumberOfMessages=1,
            WaitTimeSeconds=1,
        )
        msgs = recv.get("Messages") or []
        if not msgs:
            continue
        body = msgs[0]["Body"]
        rh = msgs[0].get("ReceiptHandle")
        if rh:
            sqs_client.delete_message(QueueUrl=queue_url, ReceiptHandle=rh)
        return body
    return ""


def test_eventbridge_deliver_sqs_without_rolearn(
    events_client, sqs_client, sts_client, unique_prefix
):
    account_id = sts_client.get_caller_identity()["Account"]
    assert account_id

    bus_name = f"{unique_prefix}-bus"
    rule_name = f"{unique_prefix}-rule"
    q_name = f"{unique_prefix}-eb-q"

    q_out = sqs_client.create_queue(QueueName=q_name)
    q_url = q_out["QueueUrl"]
    try:
        q_arn = _queue_arn(sqs_client, q_url)

        events_client.create_event_bus(Name=bus_name)
        try:
            pattern = json.dumps({"source": [EB_SOURCE]})
            events_client.put_rule(
                Name=rule_name,
                EventBusName=bus_name,
                EventPattern=pattern,
                State="ENABLED",
            )
            try:
                policy = json.dumps(
                    {
                        "Version": "2012-10-17",
                        "Statement": [
                            {
                                "Effect": "Allow",
                                "Principal": {"Service": "events.amazonaws.com"},
                                "Action": "sqs:SendMessage",
                                "Resource": q_arn,
                                "Condition": {
                                    "ArnLike": {
                                        "aws:SourceArn": (
                                            f"arn:aws:events:us-east-1:{account_id}"
                                            f":rule/{bus_name}/{rule_name}"
                                        )
                                    }
                                },
                            }
                        ],
                    }
                )
                sqs_client.set_queue_attributes(
                    QueueUrl=q_url,
                    Attributes={"Policy": policy},
                )

                put_targets = events_client.put_targets(
                    Rule=rule_name,
                    EventBusName=bus_name,
                    Targets=[{"Id": "sqs", "Arn": q_arn}],
                )
                assert put_targets.get("FailedEntryCount", 0) == 0

                _drain_queue(sqs_client, q_url)
                marker = f"eb-rolearn-{unique_prefix}"
                put_out = events_client.put_events(
                    Entries=[
                        {
                            "EventBusName": bus_name,
                            "Source": EB_SOURCE,
                            "DetailType": "RoleArnLessDelivery",
                            "Detail": json.dumps({"marker": marker}),
                        }
                    ]
                )
                assert put_out.get("FailedEntryCount", 0) == 0

                body = _receive_one_body(sqs_client, q_url, 8)
                assert EB_SOURCE in body
                assert marker in body
            finally:
                try:
                    events_client.remove_targets(
                        Rule=rule_name,
                        EventBusName=bus_name,
                        Ids=["sqs"],
                    )
                except Exception:
                    pass
                try:
                    events_client.delete_rule(
                        Name=rule_name, EventBusName=bus_name
                    )
                except Exception:
                    pass
        finally:
            try:
                events_client.delete_event_bus(Name=bus_name)
            except Exception:
                pass
    finally:
        try:
            sqs_client.delete_queue(QueueUrl=q_url)
        except Exception:
            pass


def test_eventbridge_skip_sqs_without_rolearn_or_policy(
    events_client, sqs_client, unique_prefix
):
    bus_name = f"{unique_prefix}-bus"
    rule_name = f"{unique_prefix}-rule"
    q_name = f"{unique_prefix}-deny-q"

    q_out = sqs_client.create_queue(QueueName=q_name)
    q_url = q_out["QueueUrl"]
    try:
        q_arn = _queue_arn(sqs_client, q_url)

        events_client.create_event_bus(Name=bus_name)
        try:
            events_client.put_rule(
                Name=rule_name,
                EventBusName=bus_name,
                EventPattern=json.dumps({"source": [EB_SOURCE]}),
                State="ENABLED",
            )
            try:
                put_targets = events_client.put_targets(
                    Rule=rule_name,
                    EventBusName=bus_name,
                    Targets=[{"Id": "deny", "Arn": q_arn}],
                )
                assert put_targets.get("FailedEntryCount", 0) == 0

                _drain_queue(sqs_client, q_url)
                events_client.put_events(
                    Entries=[
                        {
                            "EventBusName": bus_name,
                            "Source": EB_SOURCE,
                            "DetailType": "RoleArnLessSkip",
                            "Detail": json.dumps({"probe": "no-policy"}),
                        }
                    ]
                )
                msg = _receive_optional_body(sqs_client, q_url, 2)
                assert msg == "", f"unexpected delivery: {msg}"
            finally:
                try:
                    events_client.remove_targets(
                        Rule=rule_name,
                        EventBusName=bus_name,
                        Ids=["deny"],
                    )
                except Exception:
                    pass
                try:
                    events_client.delete_rule(
                        Name=rule_name, EventBusName=bus_name
                    )
                except Exception:
                    pass
        finally:
            try:
                events_client.delete_event_bus(Name=bus_name)
            except Exception:
                pass
    finally:
        try:
            sqs_client.delete_queue(QueueUrl=q_url)
        except Exception:
            pass
