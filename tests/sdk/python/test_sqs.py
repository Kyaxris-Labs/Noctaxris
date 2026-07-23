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
