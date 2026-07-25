"""CloudTrail CreateTrail + StartLogging S3 delivery."""

from __future__ import annotations


def test_cloudtrail_create_start_s3_delivery(
    s3_client, cloudtrail_client, account_id, unique_prefix
):
    bucket = f"{unique_prefix}-ct".lower()
    trail = f"{unique_prefix}-trail"

    s3_client.create_bucket(Bucket=bucket)
    try:
        cloudtrail_client.create_trail(Name=trail, S3BucketName=bucket)
        cloudtrail_client.start_logging(Name=trail)

        listed = s3_client.list_objects_v2(
            Bucket=bucket, Prefix=f"AWSLogs/{account_id}/CloudTrail/"
        )
        contents = listed.get("Contents") or []
        keys = [
            o["Key"]
            for o in contents
            if f"noctaxris-{trail}-" in o["Key"] or "CloudTrail/" in o["Key"]
        ]
        assert keys, f"missing CloudTrail delivery object: {contents}"
        body = s3_client.get_object(Bucket=bucket, Key=keys[0])["Body"].read()
        assert body
    finally:
        try:
            cloudtrail_client.stop_logging(Name=trail)
        except Exception:
            pass
        try:
            cloudtrail_client.delete_trail(Name=trail)
        except Exception:
            pass
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
