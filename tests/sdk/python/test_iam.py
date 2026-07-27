import csv
import io
import os

import boto3
from conftest import endpoint

TRUST = (
    '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
    '"Principal":{"Service":"lambda.amazonaws.com"},'
    '"Action":"sts:AssumeRole"}]}'
)


def test_iam_user_and_role_round_trip(iam_client, unique_prefix):
    user_name = f"{unique_prefix}-user"
    role_name = f"{unique_prefix}-role"

    iam_client.create_user(UserName=user_name)
    try:
        user = iam_client.get_user(UserName=user_name)
        assert user["User"]["UserName"] == user_name

        iam_client.create_role(
            RoleName=role_name,
            AssumeRolePolicyDocument=TRUST,
        )
        try:
            role = iam_client.get_role(RoleName=role_name)
            assert role["Role"].get("Arn"), "GetRole missing ARN"
            iam_client.delete_role(RoleName=role_name)
        finally:
            try:
                iam_client.delete_role(RoleName=role_name)
            except Exception:
                pass

        iam_client.delete_user(UserName=user_name)
    finally:
        try:
            iam_client.delete_user(UserName=user_name)
        except Exception:
            pass


def test_iam_access_key_last_used_and_credential_report(iam_client, unique_prefix):
    user_name = f"{unique_prefix}-lastused"
    region = os.environ.get("AWS_DEFAULT_REGION", "us-east-1")
    iam_client.create_user(UserName=user_name)
    access_key_id = None
    try:
        key_out = iam_client.create_access_key(UserName=user_name)
        access_key_id = key_out["AccessKey"]["AccessKeyId"]
        secret = key_out["AccessKey"]["SecretAccessKey"]

        user_sts = boto3.client(
            "sts",
            aws_access_key_id=access_key_id,
            aws_secret_access_key=secret,
            region_name=region,
            endpoint_url=endpoint(),
        )
        user_sts.get_caller_identity()

        last_used = iam_client.get_access_key_last_used(AccessKeyId=access_key_id)
        assert last_used["UserName"] == user_name
        assert last_used["AccessKeyLastUsed"]["ServiceName"] == "sts"
        assert last_used["AccessKeyLastUsed"]["Region"] == region

        gen = iam_client.generate_credential_report()
        assert str(gen.get("State", "")).upper() == "COMPLETE"

        report = iam_client.get_credential_report()
        content = report["Content"]
        if isinstance(content, memoryview):
            content = content.tobytes()
        if isinstance(content, bytes):
            csv_text = content.decode("utf-8")
        else:
            csv_text = str(content)
        rows = list(csv.reader(io.StringIO(csv_text)))
        found = any(row and row[0] == user_name for row in rows[1:])
        assert found, f"credential report missing user {user_name}: {csv_text}"
    finally:
        if access_key_id:
            try:
                iam_client.delete_access_key(
                    UserName=user_name, AccessKeyId=access_key_id
                )
            except Exception:
                pass
        try:
            iam_client.delete_user(UserName=user_name)
        except Exception:
            pass
