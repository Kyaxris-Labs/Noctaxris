"""Config StartConfigurationRecorder writes S3 history snapshot."""

from __future__ import annotations

from conftest import form_action


def test_config_history_s3_snapshot(s3_client, account_id, unique_prefix):
    bucket = f"{unique_prefix}-config".lower()
    recorder = f"{unique_prefix}-rec"
    channel = f"{unique_prefix}-ch"

    s3_client.create_bucket(Bucket=bucket)
    try:
        form_action(
            "config",
            {
                "Action": "PutConfigurationRecorder",
                "Version": "2014-11-12",
                "ConfigurationRecorder.Name": recorder,
            },
        )
        form_action(
            "config",
            {
                "Action": "PutDeliveryChannel",
                "Version": "2014-11-12",
                "DeliveryChannel.Name": channel,
                "DeliveryChannel.s3BucketName": bucket,
            },
        )
        form_action(
            "config",
            {
                "Action": "StartConfigurationRecorder",
                "Version": "2014-11-12",
                "ConfigurationRecorderName": recorder,
            },
        )

        listed = s3_client.list_objects_v2(
            Bucket=bucket, Prefix=f"AWSLogs/{account_id}/Config/"
        )
        contents = listed.get("Contents") or []
        snap_keys = [
            o["Key"]
            for o in contents
            if f"noctaxris-config-snapshot-{recorder}-" in o["Key"]
        ]
        assert snap_keys, f"missing config snapshot under AWSLogs: {contents}"
        body = s3_client.get_object(Bucket=bucket, Key=snap_keys[0])["Body"].read()
        assert body

        tracked = f"{unique_prefix}-tracked".lower()
        s3_client.create_bucket(Bucket=tracked)
        try:
            hist_xml = form_action(
                "config",
                {
                    "Action": "GetResourceConfigHistory",
                    "Version": "2014-11-12",
                    "resourceType": "AWS::S3::Bucket",
                    "resourceId": tracked,
                },
            ).decode("utf-8")
            assert tracked in hist_xml
            assert "<configurationItemStatus>OK</configurationItemStatus>" in hist_xml

            s3_client.delete_bucket(Bucket=tracked)

            hist_xml = form_action(
                "config",
                {
                    "Action": "GetResourceConfigHistory",
                    "Version": "2014-11-12",
                    "resourceType": "AWS::S3::Bucket",
                    "resourceId": tracked,
                },
            ).decode("utf-8")
            assert tracked in hist_xml
            assert (
                "<configurationItemStatus>ResourceDeleted</configurationItemStatus>"
                in hist_xml
            )
        finally:
            try:
                s3_client.delete_bucket(Bucket=tracked)
            except Exception:
                pass
    finally:
        try:
            listed = s3_client.list_objects_v2(Bucket=bucket)
            for obj in listed.get("Contents") or []:
                s3_client.delete_object(Bucket=bucket, Key=obj["Key"])
        except Exception:
            pass
        try:
            s3_client.delete_bucket(Bucket=bucket)
        except Exception:
            pass
