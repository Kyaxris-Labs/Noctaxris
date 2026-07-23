"""Full-stack lab order pipeline (advanced SDK suite).

Gate with NOCTAXRIS_ADVANCED=1. Scenario mirrors Node.js / Go fullstack suites:
IAM + KMS + S3 + DynamoDB + SQS + SNS + Lambda CRUD + EventBridge → queue/topic
+ SSM + Secrets Manager.
"""

from __future__ import annotations

import io
import json
import os
import zipfile

import pytest
from botocore.exceptions import ClientError

pytestmark = pytest.mark.skipif(
    os.environ.get("NOCTAXRIS_ADVANCED") != "1",
    reason="set NOCTAXRIS_ADVANCED=1 to run advanced fullstack suite",
)

LAMBDA_TRUST = (
    '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
    '"Principal":{"Service":"lambda.amazonaws.com"},'
    '"Action":"sts:AssumeRole"}]}'
)

ORDER_BODY = json.dumps({"orderId": "ord-1001", "sku": "widget", "qty": 2}).encode()
ORDER_ID = "ord-1001"
CONFIG_VALUE = "lab-config-v1"
SECURE_VALUE = "lab-secure-config"
SECRET_VALUE = "lab-api-key-42"
EVENT_SOURCE = "noctaxris.lab.orders"


def _minimal_python_zip() -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as zf:
        zf.writestr(
            "index.py",
            "def handler(event, context):\n    return {'ok': True}\n",
        )
    return buf.getvalue()


def _receive_one(sqs_client, queue_url, *, max_attempts=8, wait=1):
    for _ in range(max_attempts):
        recv = sqs_client.receive_message(
            QueueUrl=queue_url,
            MaxNumberOfMessages=1,
            WaitTimeSeconds=wait,
        )
        msgs = recv.get("Messages") or []
        if msgs:
            return msgs[0]
    return None


def _drain_queue(sqs_client, queue_url):
    for _ in range(20):
        recv = sqs_client.receive_message(
            QueueUrl=queue_url,
            MaxNumberOfMessages=10,
            WaitTimeSeconds=0,
        )
        msgs = recv.get("Messages") or []
        if not msgs:
            return
        for m in msgs:
            sqs_client.delete_message(
                QueueUrl=queue_url,
                ReceiptHandle=m["ReceiptHandle"],
            )


def test_fullstack_lab_order_pipeline(
    unique_prefix,
    sts_client,
    iam_client,
    kms_client,
    s3_client,
    dynamodb_client,
    sqs_client,
    sns_client,
    lambda_client,
    events_client,
    ssm_client,
    secretsmanager_client,
):
    prefix = unique_prefix
    region = os.environ.get("AWS_DEFAULT_REGION", "us-east-1")
    cleanup = []

    def add_cleanup(fn):
        cleanup.append(fn)

    try:
        # --- provision ---
        account = sts_client.get_caller_identity()["Account"]
        assert account

        key_id = kms_client.create_key(Description=f"{prefix}-lab-cmk")["KeyMetadata"][
            "KeyId"
        ]
        add_cleanup(
            lambda: kms_client.schedule_key_deletion(
                KeyId=key_id, PendingWindowInDays=7
            )
        )
        kms_client.put_key_policy(
            KeyId=key_id,
            PolicyName="default",
            Policy=json.dumps(
                {
                    "Version": "2012-10-17",
                    "Statement": [
                        {
                            "Sid": "RootAdmin",
                            "Effect": "Allow",
                            "Principal": {"AWS": f"arn:aws:iam::{account}:root"},
                            "Action": "kms:*",
                            "Resource": "*",
                        }
                    ],
                }
            ),
        )

        role_name = f"{prefix}-lambda"
        role_arn = iam_client.create_role(
            RoleName=role_name,
            AssumeRolePolicyDocument=LAMBDA_TRUST,
        )["Role"]["Arn"]
        assert role_arn

        def _delete_role():
            try:
                iam_client.delete_role_policy(
                    RoleName=role_name, PolicyName="lab-access"
                )
            except Exception:
                pass
            iam_client.delete_role(RoleName=role_name)

        add_cleanup(_delete_role)
        iam_client.put_role_policy(
            RoleName=role_name,
            PolicyName="lab-access",
            PolicyDocument=json.dumps(
                {
                    "Version": "2012-10-17",
                    "Statement": [
                        {
                            "Effect": "Allow",
                            "Action": [
                                "s3:GetObject",
                                "s3:PutObject",
                                "dynamodb:GetItem",
                                "dynamodb:PutItem",
                                "ssm:GetParameter",
                                "ssm:GetParameters",
                                "secretsmanager:GetSecretValue",
                                "kms:Decrypt",
                                "kms:Encrypt",
                                "logs:CreateLogGroup",
                                "logs:CreateLogStream",
                                "logs:PutLogEvents",
                            ],
                            "Resource": "*",
                        }
                    ],
                }
            ),
        )
        got_role = iam_client.get_role(RoleName=role_name)
        assert got_role["Role"].get("AssumeRolePolicyDocument")

        bucket = f"{prefix}-orders".lower()
        s3_client.create_bucket(Bucket=bucket)

        def _delete_bucket():
            try:
                s3_client.delete_object(Bucket=bucket, Key="order.json")
            except Exception:
                pass
            s3_client.delete_bucket(Bucket=bucket)

        add_cleanup(_delete_bucket)
        s3_client.put_bucket_encryption(
            Bucket=bucket,
            ServerSideEncryptionConfiguration={
                "Rules": [
                    {
                        "ApplyServerSideEncryptionByDefault": {
                            "SSEAlgorithm": "aws:kms",
                            "KMSMasterKeyID": key_id,
                        }
                    }
                ]
            },
        )
        s3_client.put_object(Bucket=bucket, Key="order.json", Body=ORDER_BODY)

        table = f"{prefix}-orders"
        dynamodb_client.create_table(
            TableName=table,
            AttributeDefinitions=[{"AttributeName": "orderId", "AttributeType": "S"}],
            KeySchema=[{"AttributeName": "orderId", "KeyType": "HASH"}],
            BillingMode="PAY_PER_REQUEST",
        )
        add_cleanup(lambda: dynamodb_client.delete_table(TableName=table))
        dynamodb_client.put_item(
            TableName=table,
            Item={
                "orderId": {"S": ORDER_ID},
                "sku": {"S": "widget"},
                "qty": {"N": "2"},
            },
        )

        bus_name = f"{prefix}-bus"
        rule_name = f"{prefix}-rule"

        eb_url = sqs_client.create_queue(QueueName=f"{prefix}-eb-q")["QueueUrl"]
        add_cleanup(lambda: sqs_client.delete_queue(QueueUrl=eb_url))
        eb_arn = sqs_client.get_queue_attributes(
            QueueUrl=eb_url, AttributeNames=["QueueArn"]
        )["Attributes"]["QueueArn"]
        sqs_client.set_queue_attributes(
            QueueUrl=eb_url,
            Attributes={
                "Policy": json.dumps(
                    {
                        "Version": "2012-10-17",
                        "Statement": [
                            {
                                "Effect": "Allow",
                                "Principal": {"Service": "events.amazonaws.com"},
                                "Action": "sqs:SendMessage",
                                "Resource": eb_arn,
                                "Condition": {
                                    "ArnEquals": {
                                        "aws:SourceArn": (
                                            f"arn:aws:events:{region}:{account}"
                                            f":rule/{bus_name}/{rule_name}"
                                        )
                                    }
                                },
                            }
                        ],
                    }
                )
            },
        )

        fan_url = sqs_client.create_queue(QueueName=f"{prefix}-sns-q")["QueueUrl"]
        add_cleanup(lambda: sqs_client.delete_queue(QueueUrl=fan_url))
        fan_arn = sqs_client.get_queue_attributes(
            QueueUrl=fan_url, AttributeNames=["QueueArn"]
        )["Attributes"]["QueueArn"]
        sqs_client.set_queue_attributes(
            QueueUrl=fan_url,
            Attributes={
                "Policy": json.dumps(
                    {
                        "Version": "2012-10-17",
                        "Statement": [
                            {
                                "Effect": "Allow",
                                "Principal": {"Service": "sns.amazonaws.com"},
                                "Action": "sqs:SendMessage",
                                "Resource": fan_arn,
                            }
                        ],
                    }
                )
            },
        )

        topic_arn = sns_client.create_topic(Name=f"{prefix}-orders")["TopicArn"]
        add_cleanup(lambda: sns_client.delete_topic(TopicArn=topic_arn))
        sns_client.set_topic_attributes(
            TopicArn=topic_arn,
            AttributeName="Policy",
            AttributeValue=json.dumps(
                {
                    "Version": "2012-10-17",
                    "Statement": [
                        {
                            "Effect": "Allow",
                            "Principal": {"Service": "events.amazonaws.com"},
                            "Action": "sns:Publish",
                            "Resource": topic_arn,
                        }
                    ],
                }
            ),
        )
        sub_arn = sns_client.subscribe(
            TopicArn=topic_arn, Protocol="sqs", Endpoint=fan_arn
        )["SubscriptionArn"]
        add_cleanup(lambda: sns_client.unsubscribe(SubscriptionArn=sub_arn))

        string_param = f"/lab/{prefix}/config"
        secure_param = f"/lab/{prefix}/secure"
        ssm_client.put_parameter(
            Name=string_param, Value=CONFIG_VALUE, Type="String"
        )
        add_cleanup(lambda: ssm_client.delete_parameter(Name=string_param))
        ssm_client.put_parameter(
            Name=secure_param,
            Value=SECURE_VALUE,
            Type="SecureString",
            KeyId=key_id,
        )
        add_cleanup(lambda: ssm_client.delete_parameter(Name=secure_param))

        secret_name = f"{prefix}-api-key"
        secret_arn = secretsmanager_client.create_secret(
            Name=secret_name,
            SecretString=SECRET_VALUE,
            KmsKeyId=key_id,
        )["ARN"]
        assert secret_arn
        add_cleanup(
            lambda: secretsmanager_client.delete_secret(
                SecretId=secret_name, ForceDeleteWithoutRecovery=True
            )
        )

        fn_name = f"{prefix}-fn"
        lambda_client.create_function(
            FunctionName=fn_name,
            Runtime="python3.12",
            Role=role_arn,
            Handler="index.handler",
            Code={"ZipFile": _minimal_python_zip()},
        )
        add_cleanup(lambda: lambda_client.delete_function(FunctionName=fn_name))
        fn = lambda_client.get_function(FunctionName=fn_name)
        assert fn["Configuration"]["Role"] == role_arn

        # --- wire_eventbridge ---
        events_client.create_event_bus(Name=bus_name)
        add_cleanup(lambda: events_client.delete_event_bus(Name=bus_name))
        events_client.put_rule(
            Name=rule_name,
            EventBusName=bus_name,
            EventPattern=json.dumps({"source": [EVENT_SOURCE]}),
            State="ENABLED",
        )

        def _delete_rule():
            try:
                events_client.remove_targets(
                    Rule=rule_name, EventBusName=bus_name, Ids=["sqs", "sns"]
                )
            except Exception:
                pass
            events_client.delete_rule(Name=rule_name, EventBusName=bus_name)

        add_cleanup(_delete_rule)
        events_client.put_targets(
            Rule=rule_name,
            EventBusName=bus_name,
            Targets=[
                {"Id": "sqs", "Arn": eb_arn},
                {"Id": "sns", "Arn": topic_arn},
            ],
        )
        listed = events_client.list_targets_by_rule(
            Rule=rule_name, EventBusName=bus_name
        )
        arns = [t["Arn"] for t in listed.get("Targets", [])]
        assert eb_arn in arns, "SQS target missing"
        assert topic_arn in arns, "SNS target missing"

        # --- put_events_to_sqs ---
        _drain_queue(sqs_client, eb_url)
        put = events_client.put_events(
            Entries=[
                {
                    "EventBusName": bus_name,
                    "Source": EVENT_SOURCE,
                    "DetailType": "OrderCreated",
                    "Detail": json.dumps(
                        {"orderId": ORDER_ID, "marker": "eb-order-pipeline"}
                    ),
                }
            ]
        )
        assert put.get("FailedEntryCount", 0) == 0
        msg = _receive_one(sqs_client, eb_url)
        assert msg is not None, "expected EventBridge delivery to SQS"
        body = msg["Body"]
        assert "noctaxris.lab.orders" in body
        assert "eb-order-pipeline" in body
        assert "ord-1001" in body
        sqs_client.delete_message(
            QueueUrl=eb_url, ReceiptHandle=msg["ReceiptHandle"]
        )

        events_client.put_events(
            Entries=[
                {
                    "EventBusName": bus_name,
                    "Source": "noctaxris.lab.other",
                    "DetailType": "OrderCreated",
                    "Detail": json.dumps({"orderId": "no-match"}),
                }
            ]
        )
        miss = _receive_one(sqs_client, eb_url, max_attempts=3, wait=0)
        assert miss is None, "mismatched source must not deliver"

        # --- sns_fanout ---
        _drain_queue(sqs_client, fan_url)
        marker = f"sns-fanout-{prefix}"
        pub = sns_client.publish(
            TopicArn=topic_arn, Message=marker, Subject="lab-order"
        )
        assert pub.get("MessageId")
        fan_msg = _receive_one(sqs_client, fan_url)
        assert fan_msg is not None, "expected SNS→SQS fan-out message"
        assert marker in fan_msg["Body"]
        sqs_client.delete_message(
            QueueUrl=fan_url, ReceiptHandle=fan_msg["ReceiptHandle"]
        )

        # --- data_plane_reads ---
        obj = s3_client.get_object(Bucket=bucket, Key="order.json")
        assert obj["Body"].read() == ORDER_BODY

        item = dynamodb_client.get_item(
            TableName=table, Key={"orderId": {"S": ORDER_ID}}
        )
        assert item["Item"]["sku"]["S"] == "widget"
        assert item["Item"]["qty"]["N"] == "2"

        assert (
            ssm_client.get_parameter(Name=string_param)["Parameter"]["Value"]
            == CONFIG_VALUE
        )
        assert (
            ssm_client.get_parameter(Name=secure_param, WithDecryption=True)[
                "Parameter"
            ]["Value"]
            == SECURE_VALUE
        )
        assert (
            secretsmanager_client.get_secret_value(SecretId=secret_name)[
                "SecretString"
            ]
            == SECRET_VALUE
        )

        # --- authz_denies ---
        deny_url = sqs_client.create_queue(QueueName=f"{prefix}-deny-q")["QueueUrl"]
        add_cleanup(lambda: sqs_client.delete_queue(QueueUrl=deny_url))
        deny_arn = sqs_client.get_queue_attributes(
            QueueUrl=deny_url, AttributeNames=["QueueArn"]
        )["Attributes"]["QueueArn"]
        deny_rule = f"{prefix}-deny-rule"
        events_client.put_rule(
            Name=deny_rule,
            EventBusName=bus_name,
            EventPattern=json.dumps({"source": [EVENT_SOURCE]}),
            State="ENABLED",
        )

        def _delete_deny_rule():
            try:
                events_client.remove_targets(
                    Rule=deny_rule, EventBusName=bus_name, Ids=["deny-sqs"]
                )
            except Exception:
                pass
            events_client.delete_rule(Name=deny_rule, EventBusName=bus_name)

        add_cleanup(_delete_deny_rule)
        events_client.put_targets(
            Rule=deny_rule,
            EventBusName=bus_name,
            Targets=[{"Id": "deny-sqs", "Arn": deny_arn}],
        )
        events_client.put_events(
            Entries=[
                {
                    "EventBusName": bus_name,
                    "Source": EVENT_SOURCE,
                    "DetailType": "OrderCreated",
                    "Detail": json.dumps({"marker": "should-not-land"}),
                }
            ]
        )
        blocked = _receive_one(sqs_client, deny_url, max_attempts=3, wait=0)
        assert blocked is None, (
            "delivery without events principal on queue must not land"
        )

        deny_key_id = kms_client.create_key(Description=f"{prefix}-deny-cmk")[
            "KeyMetadata"
        ]["KeyId"]
        add_cleanup(
            lambda: kms_client.schedule_key_deletion(
                KeyId=deny_key_id, PendingWindowInDays=7
            )
        )
        deny_param = f"/lab/{prefix}/kms-deny"
        ssm_client.put_parameter(
            Name=deny_param,
            Value="locked",
            Type="SecureString",
            KeyId=deny_key_id,
        )
        add_cleanup(lambda: ssm_client.delete_parameter(Name=deny_param))
        kms_client.put_key_policy(
            KeyId=deny_key_id,
            PolicyName="default",
            Policy=json.dumps(
                {
                    "Version": "2012-10-17",
                    "Statement": [
                        {
                            "Sid": "EncryptOnly",
                            "Effect": "Allow",
                            "Principal": {"AWS": f"arn:aws:iam::{account}:root"},
                            "Action": [
                                "kms:Encrypt",
                                "kms:GenerateDataKey*",
                                "kms:DescribeKey",
                            ],
                            "Resource": "*",
                        }
                    ],
                }
            ),
        )
        with pytest.raises(ClientError) as exc:
            ssm_client.get_parameter(Name=deny_param, WithDecryption=True)
        err = exc.value.response.get("Error", {})
        code = err.get("Code", "")
        msg = err.get("Message", "")
        assert (
            "AccessDenied" in code
            or "AccessDeniedException" in code
            or "AccessDenied" in msg
            or "KMS" in msg
            or "not authorized" in msg.lower()
        ), f"unexpected deny error: {err}"

    finally:
        for fn in reversed(cleanup):
            try:
                fn()
            except Exception:
                pass
