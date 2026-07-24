import json
import time


def _queue_arn(sqs_client, queue_url: str) -> str:
    attrs = sqs_client.get_queue_attributes(
        QueueUrl=queue_url,
        AttributeNames=["QueueArn"],
    )
    arn = attrs["Attributes"]["QueueArn"]
    assert arn
    return arn


def test_s3_notification_put_object_to_sqs(s3_client, sqs_client, unique_prefix):
    bucket = f"sdk-s3n-{unique_prefix}".lower()
    q_name = f"sdk-s3n-q-{unique_prefix}"
    key = "notify.txt"

    s3_client.create_bucket(Bucket=bucket)
    try:
        q_out = sqs_client.create_queue(QueueName=q_name)
        q_url = q_out["QueueUrl"]
        try:
            q_arn = _queue_arn(sqs_client, q_url)
            bucket_arn = f"arn:aws:s3:::{bucket}"
            policy = json.dumps(
                {
                    "Version": "2012-10-17",
                    "Statement": [
                        {
                            "Effect": "Allow",
                            "Principal": {"Service": "s3.amazonaws.com"},
                            "Action": "sqs:SendMessage",
                            "Resource": q_arn,
                            "Condition": {
                                "ArnLike": {"aws:SourceArn": bucket_arn}
                            },
                        }
                    ],
                }
            )
            sqs_client.set_queue_attributes(
                QueueUrl=q_url,
                Attributes={"Policy": policy},
            )

            s3_client.put_bucket_notification_configuration(
                Bucket=bucket,
                NotificationConfiguration={
                    "QueueConfigurations": [
                        {
                            "Id": "q1",
                            "QueueArn": q_arn,
                            "Events": ["s3:ObjectCreated:*"],
                        }
                    ]
                },
            )

            s3_client.put_object(
                Bucket=bucket,
                Key=key,
                Body=b"hello-notify",
            )

            body = ""
            deadline = time.time() + 10
            while time.time() < deadline:
                recv = sqs_client.receive_message(
                    QueueUrl=q_url,
                    MaxNumberOfMessages=1,
                    WaitTimeSeconds=1,
                )
                msgs = recv.get("Messages") or []
                if not msgs:
                    continue
                body = msgs[0]["Body"]
                break

            assert body, "expected S3 notification message on SQS"
            assert '"eventSource":"aws:s3"' in body
            assert bucket in body
            assert key in body
        finally:
            try:
                sqs_client.delete_queue(QueueUrl=q_url)
            except Exception:
                pass
    finally:
        try:
            s3_client.delete_object(Bucket=bucket, Key=key)
        except Exception:
            pass
        try:
            s3_client.delete_bucket(Bucket=bucket)
        except Exception:
            pass


def test_s3_notification_deny_without_queue_policy(
    s3_client, sqs_client, unique_prefix
):
    bucket = f"sdk-s3n-deny-{unique_prefix}".lower()
    q_name = f"sdk-s3n-deny-q-{unique_prefix}"
    key = "deny.txt"

    s3_client.create_bucket(Bucket=bucket)
    try:
        q_out = sqs_client.create_queue(QueueName=q_name)
        q_url = q_out["QueueUrl"]
        try:
            q_arn = _queue_arn(sqs_client, q_url)
            bucket_arn = f"arn:aws:s3:::{bucket}"
            policy = json.dumps(
                {
                    "Version": "2012-10-17",
                    "Statement": [
                        {
                            "Effect": "Allow",
                            "Principal": {"Service": "s3.amazonaws.com"},
                            "Action": "sqs:SendMessage",
                            "Resource": q_arn,
                            "Condition": {
                                "ArnLike": {"aws:SourceArn": bucket_arn}
                            },
                        }
                    ],
                }
            )
            sqs_client.set_queue_attributes(
                QueueUrl=q_url,
                Attributes={"Policy": policy},
            )

            s3_client.put_bucket_notification_configuration(
                Bucket=bucket,
                NotificationConfiguration={
                    "QueueConfigurations": [
                        {
                            "QueueArn": q_arn,
                            "Events": ["s3:ObjectCreated:Put"],
                        }
                    ]
                },
            )

            sqs_client.set_queue_attributes(
                QueueUrl=q_url,
                Attributes={"Policy": ""},
            )

            s3_client.put_object(
                Bucket=bucket,
                Key=key,
                Body=b"no-notify",
            )

            recv = sqs_client.receive_message(
                QueueUrl=q_url,
                MaxNumberOfMessages=10,
                WaitTimeSeconds=2,
            )
            assert not recv.get("Messages"), "emit must re-check queue policy"
        finally:
            try:
                sqs_client.delete_queue(QueueUrl=q_url)
            except Exception:
                pass
    finally:
        try:
            s3_client.delete_object(Bucket=bucket, Key=key)
        except Exception:
            pass
        try:
            s3_client.delete_bucket(Bucket=bucket)
        except Exception:
            pass
