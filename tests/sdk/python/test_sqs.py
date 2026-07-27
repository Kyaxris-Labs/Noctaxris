import json


def test_sqs_send_receive_delete_round_trip(sqs_client, unique_prefix):
    name = f"{unique_prefix}-q"
    created = sqs_client.create_queue(QueueName=name)
    queue_url = created["QueueUrl"]
    assert queue_url

    try:
        sqs_client.send_message(QueueUrl=queue_url, MessageBody="sdk-sqs-body")
        recv = sqs_client.receive_message(
            QueueUrl=queue_url,
            MaxNumberOfMessages=1,
            WaitTimeSeconds=1,
        )
        messages = recv.get("Messages") or []
        assert len(messages) == 1
        assert messages[0]["Body"] == "sdk-sqs-body"
        assert messages[0].get("ReceiptHandle")

        sqs_client.delete_message(
            QueueUrl=queue_url,
            ReceiptHandle=messages[0]["ReceiptHandle"],
        )
        sqs_client.delete_queue(QueueUrl=queue_url)
    finally:
        try:
            sqs_client.delete_queue(QueueUrl=queue_url)
        except Exception:
            pass


def test_sqs_redrive_policy_dlq_source_arn(sqs_client, unique_prefix):
    dlq_name = f"{unique_prefix}-dlq"
    src_name = f"{unique_prefix}-src"

    dlq = sqs_client.create_queue(QueueName=dlq_name)
    dlq_url = dlq["QueueUrl"]
    src_url = None
    try:
        dlq_attrs = sqs_client.get_queue_attributes(
            QueueUrl=dlq_url, AttributeNames=["QueueArn"]
        )
        dlq_arn = dlq_attrs["Attributes"]["QueueArn"]

        src = sqs_client.create_queue(
            QueueName=src_name,
            Attributes={
                "VisibilityTimeout": "0",
                "RedrivePolicy": json.dumps(
                    {
                        "deadLetterTargetArn": dlq_arn,
                        "maxReceiveCount": "1",
                    }
                ),
            },
        )
        src_url = src["QueueUrl"]
        src_attrs = sqs_client.get_queue_attributes(
            QueueUrl=src_url, AttributeNames=["QueueArn"]
        )
        src_arn = src_attrs["Attributes"]["QueueArn"]

        sqs_client.send_message(QueueUrl=src_url, MessageBody="poison")
        first = sqs_client.receive_message(
            QueueUrl=src_url,
            MaxNumberOfMessages=1,
            WaitTimeSeconds=1,
        )
        messages = first.get("Messages") or []
        assert len(messages) == 1
        sqs_client.change_message_visibility(
            QueueUrl=src_url,
            ReceiptHandle=messages[0]["ReceiptHandle"],
            VisibilityTimeout=0,
        )

        second = sqs_client.receive_message(
            QueueUrl=src_url,
            MaxNumberOfMessages=1,
            WaitTimeSeconds=1,
        )
        assert not (second.get("Messages") or [])

        dlq_recv = sqs_client.receive_message(
            QueueUrl=dlq_url,
            MaxNumberOfMessages=1,
            WaitTimeSeconds=1,
            MessageAttributeNames=["All"],
        )
        dlq_msgs = dlq_recv.get("Messages") or []
        assert len(dlq_msgs) == 1
        assert dlq_msgs[0]["Body"] == "poison"
        prov = (dlq_msgs[0].get("MessageAttributes") or {}).get(
            "NoctaxrisDlqSourceArn"
        )
        assert prov, f"missing NoctaxrisDlqSourceArn: {dlq_msgs[0]}"
        assert prov["StringValue"] == src_arn
        assert prov["DataType"] == "String"
    finally:
        if src_url:
            try:
                sqs_client.delete_queue(QueueUrl=src_url)
            except Exception:
                pass
        try:
            sqs_client.delete_queue(QueueUrl=dlq_url)
        except Exception:
            pass
